package cmd

import (
	"bytes"
	"crypto/rsa"
	"encoding/hex"
	"fmt"
	"os"
	"time"

	"github.com/Laisky/errors/v2"
	"github.com/Laisky/zap"
	"github.com/spf13/cobra"

	gcrypto "github.com/Laisky/go-utils/v6/crypto"
	"github.com/Laisky/go-utils/v6/internal/fileguard"
	"github.com/Laisky/go-utils/v6/log"
)

// RSA some rsa command tools
var RSA = &cobra.Command{
	Use:   "rsa",
	Short: "rsa",
	Args:  NoExtraArgs,
	Run: func(_ *cobra.Command, _ []string) {
	},
}

var (
	rsaPrikeyPemFilepath string
	rsaPubkeyPemFilepath string
	fileWantToSignature  string
)

const (
	rsaSignPrefixSHA256 = "rsa-sha256::"
)

func init() {
	rootCmd.AddCommand(RSA)

	RSA.AddCommand(RSASign)
	RSASign.PersistentFlags().StringVarP(&rsaPrikeyPemFilepath, "prikey", "p", "", "filepath of prikey in PEM format")
	RSASign.PersistentFlags().StringVarP(&fileWantToSignature, "file", "f", "", "file what to generate signature")

	RSA.AddCommand(RSAVerify)
	RSAVerify.PersistentFlags().StringVarP(&rsaPubkeyPemFilepath, "pubkey", "p", "", "filepath of pubkey in PEM format")
}

// RSASign sign file by rsa
var RSASign = &cobra.Command{
	Use:   "sign",
	Short: "sign by RSA & SHA256",
	Args:  NoExtraArgs,
	Run: func(_ *cobra.Command, _ []string) {
		err := SignFileByRSA(rsaPrikeyPemFilepath, fileWantToSignature)
		if err != nil {
			log.Shared.Panic("sign by rsa", zap.Error(err))
		}
	},
}

// RSAVerify verify file by rsa
var RSAVerify = &cobra.Command{
	Use:   "verify",
	Short: "verify by RSA & SHA256",
	Args:  NoExtraArgs,
	Run: func(_ *cobra.Command, _ []string) {
		err := VerifyFileByRSA(rsaPubkeyPemFilepath, fileWantToSignature)
		if err != nil {
			log.Shared.Panic("verify by rsa", zap.Error(err))
		}
	},
}

// VerifyFileByRSA verify file by rsa
func VerifyFileByRSA(pubkeyPath, filePath string) error {
	startAt := time.Now()
	pubkeyPem, err := os.ReadFile(pubkeyPath)
	if err != nil {
		return errors.Wrapf(err, "read pubkey %q", pubkeyPath)
	}

	pubkeyi, err := gcrypto.Pem2Pubkey(pubkeyPem)
	if err != nil {
		return errors.Wrap(err, "parse pubkey")
	}
	pubkey, ok := pubkeyi.(*rsa.PublicKey)
	if !ok {
		return errors.Errorf("pubkey must be rsa private key")
	}

	fp, err := os.Open(filePath)
	if err != nil {
		return errors.Wrapf(err, "open file %q", filePath)
	}

	sigFile := filePath + ".sig"
	sigStr, err := os.ReadFile(sigFile)
	if err != nil {
		return errors.Wrapf(err, "read signature file %q", sigFile)
	}

	sigStr = bytes.TrimPrefix(sigStr, []byte(rsaSignPrefixSHA256))
	sig, err := hex.DecodeString(string(sigStr))
	if err != nil {
		return errors.Wrap(err, "parse signature")
	}

	err = gcrypto.VerifyReaderByRSAWithSHA256(pubkey, fp, sig)
	if err != nil {
		return errors.Wrap(err, "verify signature")
	}

	log.Shared.Debug("succeed verify signature for file",
		zap.String("file", filePath),
		zap.String("sig_file", sigFile),
		zap.ByteString("sig", sigStr),
		zap.String("cost", fmt.Sprintf("%.2fs", float64(time.Since(startAt)/time.Second))),
	)

	return nil
}

// SignFileByRSA sign file by rsa
func SignFileByRSA(prikeyPath, filePath string) error {
	startAt := time.Now()
	prikeyPem, err := os.ReadFile(prikeyPath)
	if err != nil {
		return errors.Wrapf(err, "read prikey %q", prikeyPath)
	}

	prikeyi, err := gcrypto.Pem2Prikey(prikeyPem)
	if err != nil {
		return errors.Wrap(err, "parse prikey")
	}
	prikey, ok := prikeyi.(*rsa.PrivateKey)
	if !ok {
		return errors.Errorf("prikey must be rsa private key")
	}

	sigBytes, err := signFileContent(prikey, filePath)
	if err != nil {
		return errors.Wrapf(err, "generate signature")
	}

	sig := hex.EncodeToString(sigBytes)
	sig = rsaSignPrefixSHA256 + sig

	// The signature is public, so it keeps the historical 0644 creation mode. It is
	// published through a temporary file and rename: an existing regular .sig is
	// replaced, while a link or special file at the .sig path is rejected and its
	// target is never written.
	sigFile := filePath + ".sig"
	if err = fileguard.Replace(sigFile, 0, 0o644, func(sigFp *os.File) error {
		if _, err := sigFp.WriteString(sig); err != nil {
			return errors.Wrap(err, "write signature")
		}
		return nil
	}); err != nil {
		return errors.Wrapf(err, "write signature to sig file %q", sigFile)
	}

	log.Shared.Debug("succeed generate signature for file",
		zap.String("file", filePath),
		zap.String("sig_file", sigFile),
		zap.String("sig", sig),
		zap.String("cost", fmt.Sprintf("%.2fs", float64(time.Since(startAt)/time.Second))),
	)

	return nil
}

// signFileContent signs the content of filePath with prikey using RSA and SHA-256.
// It takes the private key and the input path, and returns the raw signature or an
// error; the input file is always closed.
func signFileContent(prikey *rsa.PrivateKey, filePath string) (sig []byte, retErr error) {
	fp, err := os.Open(filePath)
	if err != nil {
		return nil, errors.Wrapf(err, "open file %q", filePath)
	}
	defer func() {
		if err := fp.Close(); err != nil {
			retErr = errors.Join(retErr, errors.Wrapf(err, "close file %q", filePath))
		}
	}()

	sig, err = gcrypto.SignReaderByRSAWithSHA256(prikey, fp)
	if err != nil {
		return nil, errors.Wrap(err, "sign file content")
	}
	return sig, nil
}
