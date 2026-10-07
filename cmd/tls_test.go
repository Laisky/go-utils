package cmd

import (
	"context"
	"crypto/tls"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/zap"

	gutils "github.com/Laisky/go-utils/v6"
	gcrypto "github.com/Laisky/go-utils/v6/crypto"
	glog "github.com/Laisky/go-utils/v6/log"
)

// Test_showPemFileX509CertInfo verifies that the certinfo command prints a self-signed PEM certificate read
// from a file: it parses the -f flag into tlsInfoCMDArgs and runs tlsInfoCMD.RunE without error. It does not
// run in parallel because it mutates the shared command flags.
func Test_showPemFileX509CertInfo(t *testing.T) {
	// This test cannot run in parallel.

	_, certDer, err := gcrypto.NewRSAPrikeyAndCert(gcrypto.RSAPrikeyBits3072,
		gcrypto.WithX509CertCommonName("laisky-test"))
	require.NoError(t, err)
	certPem := gcrypto.CertDer2Pem(certDer)

	dir, err := os.MkdirTemp("", "*")
	require.NoError(t, err)
	defer os.RemoveAll(dir)

	certfile := filepath.Join(dir, "cert")
	err = os.WriteFile(certfile, certPem, 0600)
	require.NoError(t, err)

	t.Run("execute", func(t *testing.T) {
		args := []string{"", "certinfo", "-f", certfile}
		err = tlsInfoCMD.Flags().Parse(args)
		require.NoError(t, err)

		err = tlsInfoCMD.RunE(tlsInfoCMD, args)
		require.NoError(t, err)
	})
}

// Test_showRemoteX509CertInfo verifies that showRemoteX509CertInfo fetches and prints the self-signed
// certificate chain of a local TLS listener bound to an ephemeral 127.0.0.1 port, which also confirms that
// untrusted certificates are inspected without verification. The listener is closed when the test context is
// canceled.
func Test_showRemoteX509CertInfo(t *testing.T) {
	// This test cannot run in parallel.

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	prikeyPem, certDer, err := gcrypto.NewRSAPrikeyAndCert(gcrypto.RSAPrikeyBits3072,
		gcrypto.WithX509CertCommonName("laisky-test"))
	require.NoError(t, err)
	prikey, err := gcrypto.Pem2Prikey(prikeyPem)
	require.NoError(t, err)

	// Bind an ephemeral port up front so parallel runs never collide.
	listenAddr := make(chan string, 1)
	readyCtx, readyCancel := context.WithCancel(ctx)
	go func() {
		t := gutils.NewGoroutineTest(t, cancel)
		listener, err := tls.Listen("tcp", "127.0.0.1:0", &tls.Config{
			Certificates: []tls.Certificate{
				{
					Certificate: [][]byte{certDer},
					PrivateKey:  prikey,
				},
			},
		})
		require.NoError(t, err)

		go func() {
			<-ctx.Done()
			listener.Close()
		}()

		listenAddr <- listener.Addr().String()
		readyCancel()
		for {
			conn, err := listener.Accept()
			if err != nil {
				require.ErrorContains(t, err, "use of closed network connection")
				return
			}

			go func() {
				defer conn.Close()

				for {
					cnt, err := io.ReadAll(conn)
					if err != nil {
						require.ErrorContains(t, err, "use of closed network connection")
						return
					}

					glog.Shared.Debug("got", zap.ByteString("cnt", cnt))
				}
			}()
		}
	}()

	<-readyCtx.Done()
	err = showRemoteX509CertInfo(context.Background(), <-listenAddr)
	require.NoError(t, err)
}
