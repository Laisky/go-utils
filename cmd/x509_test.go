package cmd

import (
	"context"
	"crypto/tls"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	gutils "github.com/Laisky/go-utils/v4"
	gcrypto "github.com/Laisky/go-utils/v4/crypto"
)

func Test_tlsInfoCMD(t *testing.T) {
	prikeypem, certder, err := gcrypto.NewRSAPrikeyAndCert(gcrypto.RSAPrikeyBits4096)
	require.NoError(t, err)

	certPem := gcrypto.CertDer2Pem(certder)

	prikey, err := gcrypto.Pem2Prikey(prikeypem)
	require.NoError(t, err)

	// cert, err := gcrypto.Der2Cert(certder)
	// require.NoError(t, err)

	t.Run("der cert in file", func(t *testing.T) {
		dir, err := os.MkdirTemp("", "certinfo")
		require.NoError(t, err)
		defer os.RemoveAll(dir)

		certpath := filepath.Join(dir, "cert.crt")
		require.NoError(t, os.WriteFile(certpath, certder, 0600))

		tlsInfoCMDArgs.filepath = certpath
		err = tlsInfoCMD.RunE(nil, nil)
		require.NoError(t, err)
	})

	t.Run("pem cert in file", func(t *testing.T) {
		dir, err := os.MkdirTemp("", "certinfo")
		require.NoError(t, err)
		defer os.RemoveAll(dir)

		certpath := filepath.Join(dir, "cert.pem")
		require.NoError(t, os.WriteFile(certpath, certPem, 0600))

		tlsInfoCMDArgs.filepath = certpath
		err = tlsInfoCMD.RunE(nil, nil)
		require.NoError(t, err)
	})

	t.Run("cert in tls", func(t *testing.T) {
		dir, err := os.MkdirTemp("", "certinfo")
		require.NoError(t, err)
		defer os.RemoveAll(dir)

		certpath := filepath.Join(dir, "cert.crt")
		require.NoError(t, os.WriteFile(certpath, certder, 0600))

		// serve tls
		tlscfg := &tls.Config{
			Certificates: []tls.Certificate{
				{
					Certificate: [][]byte{certder},
					PrivateKey:  prikey,
				},
			},
		}

		server := &http.Server{
			Addr:      "127.0.0.1:29381",
			TLSConfig: tlscfg,
		}
		defer server.Close()

		go func() {
			_ = server.ListenAndServeTLS("", "")
		}()

		ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
		defer cancel()
		gutils.WaitTCPOpen(ctx, "127.0.0.1", 29381)

		tlsInfoCMDArgs.filepath = ""
		tlsInfoCMDArgs.remote = "127.0.0.1:29381"
		err = tlsInfoCMD.RunE(nil, nil)
		require.NoError(t, err)
	})

}
