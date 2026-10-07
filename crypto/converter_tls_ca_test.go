package crypto

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"net"
	"testing"

	"github.com/stretchr/testify/require"

	gutils "github.com/Laisky/go-utils/v6"
)

// Test_UseCaAsClientTlsCert verifies that a CA certificate issued by NewX509CertByCSR with
// WithX509SignCSRIsCA can be used as a TLS client certificate: the client presents it, chained to
// the root, to an echo TLS server on an ephemeral loopback port that requires client certificates,
// with server verification skipped, and the dial and a write must succeed.
func Test_UseCaAsClientTlsCert(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	rootprikeyPem, rootcaDer, err := NewRSAPrikeyAndCert(RSAPrikeyBits4096,
		WithX509CertCommonName("laisky-test"),
		WithX509CertIsCA(),
	)
	require.NoError(t, err)

	rootcaPrikey, err := Pem2Prikey(rootprikeyPem)
	require.NoError(t, err)

	rootca, err := Der2Cert(rootcaDer)
	require.NoError(t, err)

	rootcapool := x509.NewCertPool()
	rootcapool.AppendCertsFromPEM(CertDer2Pem(rootcaDer))

	// addrCh carries the ephemeral address the TLS server bound to, so that
	// concurrently running packages or worktrees never collide on a fixed port.
	addrCh := make(chan *net.TCPAddr, 1)
	gt := gutils.NewGoroutineTest(t, cancel)
	go func(t testing.TB) {
		prikey, err := NewRSAPrikey(RSAPrikeyBits4096)
		require.NoError(t, err)

		csrDer, err := NewX509CSR(prikey, WithX509CSRCommonName("laisky-test"))
		require.NoError(t, err)

		certDer, err := NewX509CertByCSR(rootca, rootcaPrikey, csrDer)
		require.NoError(t, err)

		ln, err := tls.Listen("tcp", "127.0.0.1:0", &tls.Config{
			RootCAs:    rootcapool,
			ClientAuth: tls.RequireAndVerifyClientCert,
			Certificates: []tls.Certificate{
				{
					Certificate: [][]byte{certDer, rootcaDer},
					PrivateKey:  prikey,
				},
			},
		})
		require.NoError(t, err)
		addrCh <- ln.Addr().(*net.TCPAddr)

		for {
			conn, err := ln.Accept()
			require.NoError(t, err)

			go func() {
				buf := make([]byte, 4096)
				for {
					defer conn.Close()

					n, err := conn.Read(buf)
					if err != nil {
						t.Logf("failed to read: %v", err)
						break
					}

					if bytes.Equal(buf, []byte("close")) {
						t.Logf("close connection")
						break
					}

					_, err = conn.Write(buf[:n])
					if err != nil {
						t.Logf("failed to write: %v", err)
						break
					}
				}
			}()
		}
	}(gt)

	var serverAddr *net.TCPAddr
	select {
	case serverAddr = <-addrCh:
	case <-ctx.Done():
		t.Fatal("tls server goroutine failed before listening")
	}
	require.NoError(t, gutils.WaitTCPOpen(ctx, serverAddr.IP.String(), serverAddr.Port))

	t.Run("use ca as client tls cert", func(t *testing.T) {
		prikey, err := NewRSAPrikey(RSAPrikeyBits4096)
		require.NoError(t, err)

		csrDer, err := NewX509CSR(prikey, WithX509CSRCommonName("laisky-test"))
		require.NoError(t, err)

		certDer, err := NewX509CertByCSR(rootca, rootcaPrikey, csrDer,
			WithX509SignCSRIsCA(),
		)
		require.NoError(t, err)

		conn, err := tls.Dial("tcp", serverAddr.String(), &tls.Config{
			RootCAs:            rootcapool,
			InsecureSkipVerify: true,
			Certificates: []tls.Certificate{
				{
					Certificate: [][]byte{certDer, rootcaDer},
					PrivateKey:  prikey,
				},
			},
		})
		require.NoError(t, err)
		defer conn.Close()

		_, err = conn.Write([]byte("hello"))
		require.NoError(t, err)
	})
}

// Test_UseCaAsServerTlsCert verifies that a CA certificate issued by NewX509CertByCSR with
// WithX509SignCSRIsCA can be used as a TLS server certificate: a client presenting a leaf
// certificate from the same root dials the echo server on an ephemeral loopback port, with server
// verification skipped, and the dial and a write must succeed.
func Test_UseCaAsServerTlsCert(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	rootprikeyPem, rootcaDer, err := NewRSAPrikeyAndCert(RSAPrikeyBits4096,
		WithX509CertCommonName("laisky-test"),
		WithX509CertIsCA(),
	)
	require.NoError(t, err)

	rootcaPrikey, err := Pem2Prikey(rootprikeyPem)
	require.NoError(t, err)

	rootca, err := Der2Cert(rootcaDer)
	require.NoError(t, err)

	rootcapool := x509.NewCertPool()
	rootcapool.AppendCertsFromPEM(CertDer2Pem(rootcaDer))

	// addrCh carries the ephemeral address the TLS server bound to, so that
	// concurrently running packages or worktrees never collide on a fixed port.
	addrCh := make(chan *net.TCPAddr, 1)
	gt := gutils.NewGoroutineTest(t, cancel)
	go func(t testing.TB) {
		prikey, err := NewRSAPrikey(RSAPrikeyBits4096)
		require.NoError(t, err)

		csrDer, err := NewX509CSR(prikey, WithX509CSRCommonName("laisky-test"))
		require.NoError(t, err)

		certDer, err := NewX509CertByCSR(rootca, rootcaPrikey, csrDer,
			WithX509SignCSRIsCA(),
		)
		require.NoError(t, err)

		// cert, err := Der2Cert(certDer)
		// require.NoError(t, err)
		// t.Logf("cert: %+v", cert)

		ln, err := tls.Listen("tcp", "127.0.0.1:0", &tls.Config{
			RootCAs:    rootcapool,
			ClientAuth: tls.RequireAndVerifyClientCert,
			Certificates: []tls.Certificate{
				{
					Certificate: [][]byte{certDer, rootcaDer},
					PrivateKey:  prikey,
				},
			},
		})
		require.NoError(t, err)
		addrCh <- ln.Addr().(*net.TCPAddr)

		for {
			conn, err := ln.Accept()
			require.NoError(t, err)

			go func() {
				buf := make([]byte, 4096)
				for {
					defer conn.Close()

					n, err := conn.Read(buf)
					if err != nil {
						t.Logf("failed to read: %v", err)
						break
					}

					if bytes.Equal(buf, []byte("close")) {
						t.Logf("close connection")
						break
					}

					_, err = conn.Write(buf[:n])
					if err != nil {
						t.Logf("failed to write: %v", err)
						break
					}
				}
			}()
		}
	}(gt)

	var serverAddr *net.TCPAddr
	select {
	case serverAddr = <-addrCh:
	case <-ctx.Done():
		t.Fatal("tls server goroutine failed before listening")
	}
	require.NoError(t, gutils.WaitTCPOpen(ctx, serverAddr.IP.String(), serverAddr.Port))

	t.Run("use leaf cert as client tls cert", func(t *testing.T) {
		prikey, err := NewRSAPrikey(RSAPrikeyBits4096)
		require.NoError(t, err)

		csrDer, err := NewX509CSR(prikey, WithX509CSRCommonName("laisky-test"))
		require.NoError(t, err)

		certDer, err := NewX509CertByCSR(rootca, rootcaPrikey, csrDer)
		require.NoError(t, err)

		conn, err := tls.Dial("tcp", serverAddr.String(), &tls.Config{
			RootCAs:            rootcapool,
			InsecureSkipVerify: true,
			Certificates: []tls.Certificate{
				{
					Certificate: [][]byte{certDer, rootcaDer},
					PrivateKey:  prikey,
				},
			},
		})
		require.NoError(t, err)
		defer conn.Close()

		peercerts := conn.ConnectionState().PeerCertificates
		t.Log(peercerts)

		_, err = conn.Write([]byte("hello"))
		require.NoError(t, err)
	})
}
