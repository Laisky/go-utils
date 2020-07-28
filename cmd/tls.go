package cmd

// =========================================
// 生成 TLS 自签名证书
//
// 支持 rsa/es
// =========================================

import (
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"os"
	"strings"
	"time"

	"github.com/Laisky/go-utils"
	"github.com/Laisky/zap"
	"github.com/spf13/cobra"
)

// GenTLS 生成 tls 证书
//
//   `go run -race cmd/main/main.go gentls --host 1.2.3.4`
//
// 注，RSA 证书没毛病，P256 的 ES 证书 Chrome 尚不支持
// inspired by https://golang.org/src/crypto/tls/generate_cert.go
var GenTLS = &cobra.Command{
	Use:   "gentls",
	Short: "generate tls cert",
	Long:  ``,
	PreRunE: func(cmd *cobra.Command, args []string) error {
		return setupTLSArgs(cmd, args)
	},
	Run: func(cmd *cobra.Command, args []string) {
		utils.Logger.Info("run generateTLSCert")
		generateTLSCert()
	},
}

func setupTLSArgs(cmd *cobra.Command, args []string) (err error) {
	if err = utils.Settings.BindPFlags(cmd.Flags()); err != nil {
		return err
	}

	return
}

func init() {
	rootCmd.AddCommand(GenTLS)

	GenTLS.Flags().String("host", "", "Comma-separated hostnames and IPs to generate a certificate for")
	GenTLS.Flags().String("start-date", "2020-01-02T15:04:05+08:00", "Creation date formatted as RFC3339")
	GenTLS.Flags().Duration("duration", 365*24*time.Hour*10, "Duration that certificate is valid for")
	GenTLS.Flags().Bool("ca", false, "whether this cert should be its own Certificate Authority")
	GenTLS.Flags().Int("rsa-bits", 2048, "Size of RSA key to generate. Ignored if --ecdsa-curve is set")
	GenTLS.Flags().String("ecdsa-curve", "", "ECDSA curve to use to generate a key. Valid values are P224, P256 (recommended), P384, P521")
	GenTLS.Flags().Bool("ed25519", false, "Generate an Ed25519 key")
}

func publicKey(priv interface{}) interface{} {
	switch k := priv.(type) {
	case *rsa.PrivateKey:
		return &k.PublicKey
	case *ecdsa.PrivateKey:
		return &k.PublicKey
	case ed25519.PrivateKey:
		return k.Public().(ed25519.PublicKey)
	default:
		return nil
	}
}

func generateTLSCert() {
	host := utils.Settings.GetString("host")
	validFrom := utils.Settings.GetString("start-date")
	validFor := utils.Settings.GetDuration("duration")
	isCA := utils.Settings.GetBool("ca")
	rsaBits := utils.Settings.GetInt("rsa-bits")
	ecdsaCurve := utils.Settings.GetString("ecdsa-curve")
	ed25519Key := utils.Settings.GetBool("ed25519")

	if len(host) == 0 {
		utils.Logger.Panic("Missing required --host parameter")
	}

	var priv interface{}
	var err error
	switch ecdsaCurve {
	case "":
		if ed25519Key {
			_, priv, err = ed25519.GenerateKey(rand.Reader)
		} else {
			priv, err = rsa.GenerateKey(rand.Reader, rsaBits)
		}
	case "P224":
		priv, err = ecdsa.GenerateKey(elliptic.P224(), rand.Reader)
	case "P256":
		priv, err = ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	case "P384":
		priv, err = ecdsa.GenerateKey(elliptic.P384(), rand.Reader)
	case "P521":
		priv, err = ecdsa.GenerateKey(elliptic.P521(), rand.Reader)
	default:
		utils.Logger.Panic("Unrecognized elliptic curve", zap.String("ecdsaCurve", ecdsaCurve))
	}
	if err != nil {
		utils.Logger.Panic("Failed to generate private key", zap.Error(err))
	}

	var notBefore time.Time
	if len(validFrom) == 0 {
		notBefore = time.Now()
	} else {
		notBefore, err = time.Parse(time.RFC3339, validFrom)
		if err != nil {
			utils.Logger.Panic("Failed to parse creation date", zap.Error(err))
		}
	}

	notAfter := notBefore.Add(validFor)
	serialNumberLimit := new(big.Int).Lsh(big.NewInt(1), 128)
	serialNumber, err := rand.Int(rand.Reader, serialNumberLimit)
	if err != nil {
		utils.Logger.Panic("Failed to generate serial number", zap.Error(err))
	}

	template := x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			CommonName:   host,
			Organization: []string{"Acme Co"},
		},
		NotBefore: notBefore,
		NotAfter:  notAfter,

		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
	}

	hosts := strings.Split(host, ",")
	for _, h := range hosts {
		if ip := net.ParseIP(h); ip != nil {
			template.IPAddresses = append(template.IPAddresses, ip)
		} else {
			template.DNSNames = append(template.DNSNames, h)
		}
	}

	if isCA {
		template.IsCA = true
		template.KeyUsage |= x509.KeyUsageCertSign
	}

	derBytes, err := x509.CreateCertificate(rand.Reader, &template, &template, publicKey(priv), priv)
	if err != nil {
		utils.Logger.Panic("Failed to create certificate: %v", zap.Error(err))
	}

	certOut, err := os.Create("cert.pem")
	if err != nil {
		utils.Logger.Panic("Failed to open cert.pem for writing: %v", zap.Error(err))
	}
	if err := pem.Encode(certOut, &pem.Block{Type: "CERTIFICATE", Bytes: derBytes}); err != nil {
		utils.Logger.Panic("Failed to write data to cert.pem: %v", zap.Error(err))
	}
	if err := certOut.Close(); err != nil {
		utils.Logger.Panic("Error closing cert.pem: %v", zap.Error(err))
	}
	utils.Logger.Info("wrote cert.pem")

	keyOut, err := os.OpenFile("key.pem", os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		utils.Logger.Panic("Failed to open key.pem for writing: %v", zap.Error(err))
		return
	}
	privBytes, err := x509.MarshalPKCS8PrivateKey(priv)
	if err != nil {
		utils.Logger.Panic("Unable to marshal private key: %v", zap.Error(err))
	}
	if err := pem.Encode(keyOut, &pem.Block{Type: "PRIVATE KEY", Bytes: privBytes}); err != nil {
		utils.Logger.Panic("Failed to write data to key.pem: %v", zap.Error(err))
	}
	if err := keyOut.Close(); err != nil {
		utils.Logger.Panic("Error closing key.pem: %v", zap.Error(err))
	}

	utils.Logger.Info("wrote key.pem")
}
