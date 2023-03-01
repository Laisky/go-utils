package cmd

import (
	"bytes"
	"crypto/rsa"
	"encoding/hex"
	"fmt"
	"os"
	"time"

	"github.com/Laisky/errors"
	"github.com/Laisky/zap"
	"github.com/spf13/cobra"

	gcrypto "github.com/Laisky/go-utils/v4/crypto"
	"github.com/Laisky/go-utils/v4/log"
)

// RSA some rsa command tools
var RSA = &cobra.Command{
	Use:   "rsa",
	Short: "rsa",
	Args:  NoExtraArgs,
	Run: func(cmd *cobra.Command, args []string) {
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
	Run: func(cmd *cobra.Command, args []string) {
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
	Run: func(cmd *cobra.Command, args []string) {
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

	fp, err := os.Open(filePath)
	if err != nil {
		return errors.Wrapf(err, "open file %q", filePath)
	}

	sigFile := filePath + ".sig"
	//nolint:gosec // G302: Expect file permissions to be 0600 or less
	sigFp, err := os.OpenFile(sigFile, os.O_CREATE|os.O_RDWR|os.O_TRUNC, 0644)
	if err != nil {
		return errors.Wrapf(err, "generate signature file %q", sigFile)
	}

	sigBytes, err := gcrypto.SignReaderByRSAWithSHA256(prikey, fp)
	if err != nil {
		return errors.Wrapf(err, "generate signature")
	}

	sig := hex.EncodeToString(sigBytes)
	sig = rsaSignPrefixSHA256 + sig

	_, err = sigFp.Write([]byte(sig))
	if err != nil {
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
