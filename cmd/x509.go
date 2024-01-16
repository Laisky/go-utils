package cmd

// =========================================
// 生成 TLS 自签名证书
//
// 支持 rsa/es
// =========================================

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/Laisky/errors/v2"
	"github.com/spf13/cobra"

	gutils "github.com/Laisky/go-utils/v4"
	gcrypto "github.com/Laisky/go-utils/v4/crypto"
)

func init() {
	tlsInfoCMD.Flags().StringVarP(&tlsInfoCMDArgs.remote, "remote", "r", "", "remote tcp endpoint")
	tlsInfoCMD.Flags().StringVarP(&tlsInfoCMDArgs.filepath, "file", "f", "", "certificates file in PEM")
	rootCmd.AddCommand(tlsInfoCMD)

	csrInfoCMD.Flags().StringVarP(&csrfilepath, "file", "f", "", "csr file")
	rootCmd.AddCommand(csrInfoCMD)
}

var tlsInfoCMDArgs = struct {
	filepath string
	remote   string
}{}

// tlsInfoCMD 查询证书信息
var tlsInfoCMD = &cobra.Command{
	Use:   "certinfo",
	Short: "show x509 cert info",
	Long: gutils.Dedent(`
		Show details of x509 certificates chain for TCP endpoint or PEM file.

		Install:

	      go install github.com/Laisky/go-utils/v4/cmd/gutils@latest

		Examples:

		  - for TCP endpoint:

		    gutils certinfo -r blog.laisky.com:443

		  - for PEM file:

		    gutils certinfo -f ./cert.pem
	`),
	Args: NoExtraArgs,
	RunE: func(_ *cobra.Command, _ []string) error {
		isRemote := tlsInfoCMDArgs.remote != ""
		isFile := tlsInfoCMDArgs.filepath != ""
		var err error
		switch {
		case isRemote && isFile:
			return errors.Errorf("--remote or --file should not appears at the same time")
		case isRemote:
			err = errors.Wrap(showRemoteX509CertInfo(tlsInfoCMDArgs.remote), "show remote cert")
		case isFile:
			err = errors.Wrap(showFileX509CertInfo(tlsInfoCMDArgs.filepath), "show file cert")
		}

		if err != nil {
			return errors.Wrap(err, "show cert info")
		}

		return nil
	},
}

func showRemoteX509CertInfo(addr string) error {
	conn, err := tls.Dial("tcp", addr, &tls.Config{
		InsecureSkipVerify: true,
	})
	if err != nil {
		return errors.Wrapf(err, "dial addr %q", addr)
	}

	return prettyPrintCerts(conn.ConnectionState().PeerCertificates)
}

func showFileX509CertInfo(fpath string) error {
	certsPem, err := os.ReadFile(fpath)
	if err != nil {
		return errors.Wrapf(err, "read file %q", fpath)
	}

	certs, err := gcrypto.Pem2Certs(certsPem)
	if err != nil {
		if strings.Contains(err.Error(), "pem format invalid") {
			// cert is not in pem format, try to parse it as der
			if certs, err = gcrypto.Der2Certs(certsPem); err != nil {
				return errors.Wrap(err, "parse certs in der format")
			}
		} else {
			return errors.Wrap(err, "parse certs")
		}
	}

	return prettyPrintCerts(certs)
}

func prettyPrintCerts(certs []*x509.Certificate) error {
	parsedCerts := make([]map[string]any, 0, len(certs))
	for i := range certs {
		rc, err := gcrypto.ReadableX509Cert(certs[i])
		if err != nil {
			return errors.Wrap(err, "readable cert")
		}

		parsedCerts = append(parsedCerts, rc)
	}

	out, err := json.MarshalIndent(parsedCerts, "", "    ")
	if err != nil {
		return errors.Wrap(err, "marshal cert")
	}

	fmt.Println(string(out))
	return nil
}

var csrfilepath string

var csrInfoCMD = &cobra.Command{
	Use:   "csrinfo",
	Short: "show x509 cert request info",
	Long: gutils.Dedent(`
		Show details of x509 certificate request.

		Examples:

		  gutils csrinfo -f ./csr.pem

		file coulld be in DER or base64 format.
	`),
	Args: NoExtraArgs,
	RunE: func(_ *cobra.Command, _ []string) error {
		payload, err := os.ReadFile(csrfilepath)
		if err != nil {
			return errors.Wrapf(err, "read file %q", csrfilepath)
		}

		// try read pem
		csr, err := gcrypto.Pem2CSR(payload)

		// try decode by base64
		if err != nil {
			decodedPayload, err := base64.StdEncoding.DecodeString(string(payload))
			if err == nil { // raw content is base64 encoded
				payload = decodedPayload
			}
		}

		if err != nil {
			csr, err = gcrypto.Der2CSR(payload)
		}

		if err != nil {
			return errors.Wrap(err, "parse csr")
		}

		csrm, err := gcrypto.ReadableX509CSR(csr)
		if err != nil {
			return errors.Wrap(err, "readable csr")
		}

		output, err := json.MarshalIndent(csrm, "", "    ")
		if err != nil {
			return errors.Wrap(err, "marshal csr")
		}

		fmt.Println(string(output))
		return nil
	},
}
