package crypto

import (
	"context"
	"crypto"
	"crypto/tls"
	"crypto/x509"
	"io"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// mtlsTestTimeout bounds every dial, handshake, and echo in the mutual-TLS tests.
const mtlsTestTimeout = 30 * time.Second

// mtlsTestCA is a throwaway certificate authority used to issue mutual-TLS test certificates.
type mtlsTestCA struct {
	cert    *x509.Certificate
	certDer []byte
	prikey  crypto.PrivateKey
	pool    *x509.CertPool
}

// newMTLSTestCA creates a self-signed ECDSA P-256 root CA named commonName with this package's
// certificate helpers. It returns the CA together with a pool that trusts only that CA.
func newMTLSTestCA(t *testing.T, commonName string) *mtlsTestCA {
	t.Helper()

	prikeyPem, certDer, err := NewECDSAPrikeyAndCert(ECDSACurveP256,
		WithX509CertCommonName(commonName),
		WithX509CertIsCA(),
	)
	require.NoError(t, err)

	prikey, err := Pem2Prikey(prikeyPem)
	require.NoError(t, err)
	cert, err := Der2Cert(certDer)
	require.NoError(t, err)

	pool := x509.NewCertPool()
	pool.AddCert(cert)
	return &mtlsTestCA{cert: cert, certDer: certDer, prikey: prikey, pool: pool}
}

// issue signs a fresh ECDSA P-256 key for commonName through NewX509CSR and NewX509CertByCSR,
// passing csrOpts to the CSR and signOpts to the signer. It returns a tls.Certificate whose chain
// is the issued certificate followed by the CA certificate.
func (ca *mtlsTestCA) issue(t *testing.T, commonName string,
	csrOpts []X509CSROption, signOpts ...SignCSROption) tls.Certificate {
	t.Helper()

	prikey, err := NewECDSAPrikey(ECDSACurveP256)
	require.NoError(t, err)

	csrDer, err := NewX509CSR(prikey, append([]X509CSROption{WithX509CSRCommonName(commonName)}, csrOpts...)...)
	require.NoError(t, err)

	certDer, err := NewX509CertByCSR(ca.cert, ca.prikey, csrDer, signOpts...)
	require.NoError(t, err)

	leaf, err := Der2Cert(certDer)
	require.NoError(t, err)
	return tls.Certificate{
		Certificate: [][]byte{certDer, ca.certDer},
		PrivateKey:  prikey,
		Leaf:        leaf,
	}
}

// mtlsHandshakeResult records what the echo server observed for one accepted connection.
type mtlsHandshakeResult struct {
	// err is the server-side handshake error, nil when the client was accepted.
	err error
	// verifiedChains are the client certificate chains the server verified.
	verifiedChains [][]*x509.Certificate
}

// startMTLSEchoServer starts a TLS echo server on an ephemeral loopback port. It presents
// serverCert and requires a client certificate that verifies against clientCAs. It returns the
// listen address and a channel that receives one handshake result per accepted connection. The
// listener, every accepted connection, and the serving goroutines are torn down via t.Cleanup.
func startMTLSEchoServer(t *testing.T, serverCert tls.Certificate,
	clientCAs *x509.CertPool) (addr string, results <-chan mtlsHandshakeResult) {
	t.Helper()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	tlsLn := tls.NewListener(ln, &tls.Config{
		MinVersion:   tls.VersionTLS12,
		Certificates: []tls.Certificate{serverCert},
		ClientCAs:    clientCAs,
		ClientAuth:   tls.RequireAndVerifyClientCert,
	})

	resultCh := make(chan mtlsHandshakeResult, 16)
	var (
		wg    sync.WaitGroup
		mu    sync.Mutex
		conns []net.Conn
	)
	t.Cleanup(func() {
		require.NoError(t, tlsLn.Close())
		mu.Lock()
		for _, conn := range conns {
			_ = conn.Close() // the echo goroutine may already have closed it
		}
		mu.Unlock()
		wg.Wait()
	})

	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			conn, err := tlsLn.Accept()
			if err != nil {
				return // the listener was closed by t.Cleanup
			}
			mu.Lock()
			conns = append(conns, conn)
			mu.Unlock()

			wg.Add(1)
			go func() {
				defer wg.Done()
				serveMTLSEcho(t, conn, resultCh)
			}()
		}
	}()

	return ln.Addr().String(), resultCh
}

// serveMTLSEcho completes the server side of the TLS handshake on conn, reports the outcome on
// results, and then echoes every byte it reads until the peer closes the connection or an error
// occurs. It closes conn before returning.
func serveMTLSEcho(t *testing.T, conn net.Conn, results chan<- mtlsHandshakeResult) {
	defer func() { _ = conn.Close() }() // also closed by t.Cleanup; the second close may fail

	tlsConn, ok := conn.(*tls.Conn)
	if !ok {
		results <- mtlsHandshakeResult{err: io.ErrUnexpectedEOF}
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), mtlsTestTimeout)
	defer cancel()
	if err := tlsConn.HandshakeContext(ctx); err != nil {
		results <- mtlsHandshakeResult{err: err}
		return
	}
	results <- mtlsHandshakeResult{verifiedChains: tlsConn.ConnectionState().VerifiedChains}

	if _, err := io.Copy(tlsConn, tlsConn); err != nil {
		t.Logf("echo server stopped: %v", err)
	}
}

// mtlsRoundTrip dials addr with clientCfg, writes msg, and reads the echoed reply. It returns the
// client connection state and the reply, or the first error. Under TLS 1.3 a server that rejects
// the client certificate is only observed by the client when it reads, so a successful echo is
// the proof that the server accepted the client. The connection is closed via t.Cleanup.
func mtlsRoundTrip(t *testing.T, addr string, clientCfg *tls.Config,
	msg []byte) (tls.ConnectionState, []byte, error) {
	t.Helper()

	ctx, cancel := context.WithTimeout(t.Context(), mtlsTestTimeout)
	defer cancel()

	dialer := &tls.Dialer{Config: clientCfg}
	conn, err := dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		return tls.ConnectionState{}, nil, err
	}
	t.Cleanup(func() { _ = conn.Close() }) // the peer may already have torn the connection down

	tlsConn, ok := conn.(*tls.Conn)
	require.True(t, ok)
	require.NoError(t, tlsConn.SetDeadline(time.Now().Add(mtlsTestTimeout)))

	if _, err = tlsConn.Write(msg); err != nil {
		return tlsConn.ConnectionState(), nil, err
	}
	reply := make([]byte, len(msg))
	if _, err = io.ReadFull(tlsConn, reply); err != nil {
		return tlsConn.ConnectionState(), nil, err
	}

	return tlsConn.ConnectionState(), reply, nil
}

// receiveMTLSResult waits for the next server-side handshake result, failing t on timeout.
func receiveMTLSResult(t *testing.T, results <-chan mtlsHandshakeResult) mtlsHandshakeResult {
	t.Helper()

	select {
	case res := <-results:
		return res
	case <-time.After(mtlsTestTimeout):
		t.Fatal("timed out waiting for the server handshake result")
		return mtlsHandshakeResult{}
	}
}

// requireMTLSRejectedUnknownCA asserts that a client presenting clientCert to the echo server at
// addr fails, and that the server rejected the client certificate as issued by an unknown CA. The
// client always sends clientCert, as a hostile client would, even when its issuer is not among
// the CAs the server advertises.
func requireMTLSRejectedUnknownCA(t *testing.T, addr string, results <-chan mtlsHandshakeResult,
	serverRoots *x509.CertPool, clientCert tls.Certificate) {
	t.Helper()

	_, reply, err := mtlsRoundTrip(t, addr, &tls.Config{
		MinVersion: tls.VersionTLS12,
		RootCAs:    serverRoots,
		GetClientCertificate: func(*tls.CertificateRequestInfo) (*tls.Certificate, error) {
			return &clientCert, nil
		},
	}, []byte("hello"))
	require.Error(t, err, "a client certificate from an unrelated CA must be rejected")
	require.Nil(t, reply)

	res := receiveMTLSResult(t, results)
	require.Error(t, res.err)
	var unknownAuthority x509.UnknownAuthorityError
	require.ErrorAs(t, res.err, &unknownAuthority)
}

// Test_UseCaAsClientTlsCert verifies over real mutual TLS that a CA certificate issued by
// NewX509CertByCSR with WithX509SignCSRIsCA works as a TLS client certificate. The server requires
// and verifies client certificates against a pool holding only the root (ClientCAs), and the
// client verifies the server against the same root (RootCAs). A client certificate from the root
// completes an echo round-trip and yields a verified chain at the server, while client
// certificates issued by an unrelated CA or by an impostor CA with the root's subject name, and a
// client without a certificate, are all rejected.
func Test_UseCaAsClientTlsCert(t *testing.T) {
	t.Parallel()

	root := newMTLSTestCA(t, "mtls-root")
	serverCert := root.issue(t, "mtls-server",
		[]X509CSROption{WithX509CSRIPAddrs(net.IPv4(127, 0, 0, 1))})
	addr, results := startMTLSEchoServer(t, serverCert, root.pool)

	t.Run("ca cert issued by trusted root is accepted", func(t *testing.T) {
		clientCert := root.issue(t, "mtls-client-ca", nil, WithX509SignCSRIsCA())
		require.True(t, clientCert.Leaf.IsCA)

		state, reply, err := mtlsRoundTrip(t, addr, &tls.Config{
			MinVersion:   tls.VersionTLS12,
			RootCAs:      root.pool,
			Certificates: []tls.Certificate{clientCert},
		}, []byte("hello"))
		require.NoError(t, err)
		require.Equal(t, []byte("hello"), reply)
		require.NotEmpty(t, state.VerifiedChains, "the client must verify the server")

		res := receiveMTLSResult(t, results)
		require.NoError(t, res.err)
		require.NotEmpty(t, res.verifiedChains, "the server must verify the client")
		require.Equal(t, "mtls-client-ca", res.verifiedChains[0][0].Subject.CommonName)
		require.True(t, res.verifiedChains[0][len(res.verifiedChains[0])-1].Equal(root.cert))
	})

	t.Run("ca cert issued by unrelated root is rejected", func(t *testing.T) {
		unrelated := newMTLSTestCA(t, "mtls-unrelated-root")
		clientCert := unrelated.issue(t, "mtls-client-ca", nil, WithX509SignCSRIsCA())
		requireMTLSRejectedUnknownCA(t, addr, results, root.pool, clientCert)
	})

	t.Run("ca cert issued by impostor root with the same name is rejected", func(t *testing.T) {
		impostor := newMTLSTestCA(t, root.cert.Subject.CommonName)
		require.Equal(t, root.cert.RawSubject, impostor.cert.RawSubject)
		clientCert := impostor.issue(t, "mtls-client-ca", nil, WithX509SignCSRIsCA())
		requireMTLSRejectedUnknownCA(t, addr, results, root.pool, clientCert)
	})

	t.Run("missing client cert is rejected", func(t *testing.T) {
		_, reply, err := mtlsRoundTrip(t, addr, &tls.Config{
			MinVersion: tls.VersionTLS12,
			RootCAs:    root.pool,
		}, []byte("hello"))
		require.Error(t, err)
		require.Nil(t, reply)

		res := receiveMTLSResult(t, results)
		require.ErrorContains(t, res.err, "didn't provide a certificate")
	})
}

// Test_UseCaAsServerTlsCert verifies over real mutual TLS that a CA certificate issued by
// NewX509CertByCSR with WithX509SignCSRIsCA works as a TLS server certificate. The client verifies
// the CA-flagged server certificate against the root (RootCAs) instead of skipping verification,
// and the server requires and verifies the client's leaf certificate against the same root
// (ClientCAs). A leaf from the root completes an echo round-trip; a client that trusts only an
// unrelated root rejects the server, and a client leaf issued by an unrelated CA is rejected.
func Test_UseCaAsServerTlsCert(t *testing.T) {
	t.Parallel()

	root := newMTLSTestCA(t, "mtls-root")
	serverCert := root.issue(t, "mtls-server-ca",
		[]X509CSROption{WithX509CSRIPAddrs(net.IPv4(127, 0, 0, 1))},
		WithX509SignCSRIsCA())
	require.True(t, serverCert.Leaf.IsCA)
	addr, results := startMTLSEchoServer(t, serverCert, root.pool)

	t.Run("leaf cert issued by trusted root is accepted", func(t *testing.T) {
		clientCert := root.issue(t, "mtls-client-leaf", nil)
		require.False(t, clientCert.Leaf.IsCA)

		state, reply, err := mtlsRoundTrip(t, addr, &tls.Config{
			MinVersion:   tls.VersionTLS12,
			RootCAs:      root.pool,
			Certificates: []tls.Certificate{clientCert},
		}, []byte("hello"))
		require.NoError(t, err)
		require.Equal(t, []byte("hello"), reply)
		require.NotEmpty(t, state.VerifiedChains)
		require.Equal(t, "mtls-server-ca", state.VerifiedChains[0][0].Subject.CommonName)
		require.True(t, state.VerifiedChains[0][0].IsCA)

		res := receiveMTLSResult(t, results)
		require.NoError(t, res.err)
		require.NotEmpty(t, res.verifiedChains)
		require.Equal(t, "mtls-client-leaf", res.verifiedChains[0][0].Subject.CommonName)
	})

	t.Run("client that does not trust the root rejects the server", func(t *testing.T) {
		unrelated := newMTLSTestCA(t, "mtls-unrelated-root")
		clientCert := root.issue(t, "mtls-client-leaf", nil)

		_, reply, err := mtlsRoundTrip(t, addr, &tls.Config{
			MinVersion:   tls.VersionTLS12,
			RootCAs:      unrelated.pool,
			Certificates: []tls.Certificate{clientCert},
		}, []byte("hello"))
		require.Error(t, err)
		require.Nil(t, reply)
		var unknownAuthority x509.UnknownAuthorityError
		require.ErrorAs(t, err, &unknownAuthority)

		// The server sees the client abort the handshake.
		res := receiveMTLSResult(t, results)
		require.Error(t, res.err)
	})

	t.Run("leaf cert issued by unrelated root is rejected", func(t *testing.T) {
		unrelated := newMTLSTestCA(t, "mtls-unrelated-root")
		clientCert := unrelated.issue(t, "mtls-client-leaf", nil)
		requireMTLSRejectedUnknownCA(t, addr, results, root.pool, clientCert)
	})
}
