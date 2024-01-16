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
	t.Parallel()

	prikeypem, certder, err := gcrypto.NewRSAPrikeyAndCert(gcrypto.RSAPrikeyBits4096,
		gcrypto.WithX509CertCommonName("laisky-test"),
	)
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

func Test_csrInfo(t *testing.T) {
	t.Parallel()

	dir, err := os.MkdirTemp("", "Test_csrInfo-*")
	require.NoError(t, err)
	defer os.RemoveAll(dir)

	csrfilepath = filepath.Join(dir, "csr.der")

	t.Run("csr der in base64", func(t *testing.T) {
		err := os.WriteFile(csrfilepath, []byte("MIIEiTCCAnECAQAwEzERMA8GA1UEAxMIcGtpLXRlc3QwggIiMA0GCSqGSIb3DQEBAQUAA4ICDwAwggIKAoICAQDQPwhbHtcoyQU8QjlAOyyyNibGqArOVnkI4wD2PZSEbreDgu/4Pr+cQePukbF8V7NrJSyTjuXagtVxfXsNVa7awRpZ5COl19HElW77jfaHDWTtxjHggFwOVGBS3m80l7FYbdrDiebEZpRFVRzNNObCazdlsFXH7NHRDLCGms75raf3bDv5EmwWX8KorGxi/9xASb5aV0D5D6xd9fTA5f1Kk6HCKhU1KQBqcOPdJeLKKi29tUgZ2sARdDaN7JRIH19IoaqsXo+OGh7e30nmKZibDm8BQzJX3d+0bGGgHuPXRVO3YXL6eyCdBvzg483sLyyVoAr9zkq1ovaF6xSyDNq0pneGpWjrjww6J5wMQB5/0yk6JMxmcCfgVvxkSm1BRdutDpaWl3hJlYTE5FnJIPaDvL6ARkQpbETbAKJzUtQehDm0SKduorjByv0J/JPNdasWu1tJD7Ys2WDtCiEue+JLO0BGNSu45stcTt7F+B5DuANAo/vkLXnhSTY1rOoX4vSiW8yJGrbRg5CatlDOxmzXgetHuSmasFL9f2mS1LtQbzZpoCs7g1I6jrvUfvxjPrs3YweKXDqSWwW2Q0QjHIPou/jmxrs8KMyQsjnKOKO89muVs3jN3PTLMF1vy62AzWiaN9VNB6JXpML7tiHS3mP02N6dwqnuheyJdLAMuxeQNQIDAQABoDEwLwYJKoZIhvcNAQkOMSIwIDAeBgNVHREEFzAVghNwa2ktdGVzdC5sYWlza3kuY29tMA0GCSqGSIb3DQEBCwUAA4ICAQARE3nY0VV0RyengXj8vWfsXiZX1/+KrRVGKktAy1GwwuO065UZs8omuyNiqcJOKU/tu6vHibl/uPIxYE8yETHQUQcFN6ppC8riwthXHS1LD26+j6g501yh8CwHDAadyfGmn1d8XFRPE+Wf6JLdU2Q/pCJBmIMznVLo3iQ2EYWTRxxa0dQ7bbR9iArnns+/doj588UfuagAHH+MK0sGZVFhXVZ2XtZz8wk9bmHgzpeM7JxZ1tnG7pGps1fsGmgM9ZGvjDd/ZB6gPRbWBLltl8m2eSVSGLXYSLoil3LkrztbyRLkKfU5FClDcdXxhUIyDfVkXsR38nobpyd87OwkhlKEY9QdaaWk8gfxu/HORVz2qS9DZPWxdzXUlQRmjF05OgvGRLYmQnbhZWDNL/N/0DGYaY/E+/myfAPz/11axC8STX22pi7dIMVw5hdarCBi0VRcQ2lhIogsU8TVdOZIbUALT6epc2lM4SIwFtnztoU8PdVCeMm0bTYL1jdV5uib0j1zCFHu75HH0kbS5UTDEaZEXInLcDWVBm8Eu1tY85NWMuVWeBqY9MELGvvqiAxX324CHbaZsp1/sk8c9Y4LbFJOusK7Is8qQg45zOfcqv7FqjIHDgWn+4mGR3LKwPq4YeOtVxkzfEgE3KYKyvY4v2TMa4Ze2AuTmD/jOMfPgtUr1w=="), 0o600)
		require.NoError(t, err)

		err = csrInfoCMD.RunE(nil, nil)
		require.NoError(t, err)
	})

}
