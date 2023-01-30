package cmd

import (
	"context"
	"crypto/tls"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	gutils "github.com/Laisky/go-utils/v3"
	gencrypt "github.com/Laisky/go-utils/v3/encrypt"
)

func Test_showPemFileX509CertInfo(t *testing.T) {
	_, certDer, err := gencrypt.NewRSAPrikeyAndCert(gencrypt.RSAPrikeyBits3072)
	require.NoError(t, err)
	certPem := gencrypt.CertDer2Pem(certDer)

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
		tlsInfoCMD.Run(tlsInfoCMD, args)
	})
}

func Test_showRemoteX509CertInfo(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	prikeyPem, certDer, err := gencrypt.NewRSAPrikeyAndCert(gencrypt.RSAPrikeyBits3072)
	require.NoError(t, err)
	prikey, err := gencrypt.Pem2Prikey(prikeyPem)
	require.NoError(t, err)

	readyCtx, readyCancel := context.WithCancel(ctx)
	go func() {
		t := gutils.NewGoroutineTest(t, cancel)
		listener, err := tls.Listen("tcp", "127.0.0.1:39481", &tls.Config{
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

					t.Logf("got %q", string(cnt))
				}
			}()
		}
	}()

	<-readyCtx.Done()
	err = showRemoteX509CertInfo("127.0.0.1:39481")
	require.NoError(t, err)
}
