package cmd

// =========================================
// 生成 TLS 自签名证书
//
// 支持 rsa/es
// =========================================

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"os"

	"github.com/Laisky/errors"
	"github.com/Laisky/zap"
	"github.com/spf13/cobra"

	gutils "github.com/Laisky/go-utils/v4"
	gcrypto "github.com/Laisky/go-utils/v4/crypto"
	"github.com/Laisky/go-utils/v4/log"
)

var (
	tlsInfoCMDArgRemote string
	tlsInfoCMDArgFile   string
)

func init() {
	rootCmd.AddCommand(tlsInfoCMD)

	tlsInfoCMD.Flags().StringVarP(&tlsInfoCMDArgRemote, "remote", "r", "", "remote tcp endpoint")
	tlsInfoCMD.Flags().StringVarP(&tlsInfoCMDArgFile, "file", "f", "", "certificates file in PEM")
}

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
	Run: func(cmd *cobra.Command, args []string) {
		isRemote := tlsInfoCMDArgRemote != ""
		isFile := tlsInfoCMDArgFile != ""
		var err error
		switch {
		case isRemote && isFile:
			log.Shared.Panic("--remote or --file should not appears at the same time")
		case isRemote:
			err = errors.Wrap(showRemoteX509CertInfo(tlsInfoCMDArgRemote), "show remote cert")
		case isFile:
			err = errors.Wrap(showPemFileX509CertInfo(tlsInfoCMDArgFile), "show file cert")
		}

		if err != nil {
			log.Shared.Panic("show cert", zap.Error(err))
		}
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

func showPemFileX509CertInfo(fpath string) error {
	certsPem, err := os.ReadFile(fpath)
	if err != nil {
		return errors.Wrapf(err, "read file %q", fpath)
	}

	certs, err := gcrypto.Pem2Certs(certsPem)
	if err != nil {
		return errors.Wrap(err, "parse certs")
	}

	return prettyPrintCerts(certs)
}

func prettyPrintCerts(certs []*x509.Certificate) error {
	var parsedCerts []map[string]any
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
