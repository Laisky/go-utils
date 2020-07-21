package cmd

// =====================================
// Encrypt File
//
// 1. encrypt file by aes
// =====================================

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"github.com/Laisky/go-utils"
	"github.com/Laisky/zap"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
)

var encryptCMD = &cobra.Command{
	Use:  "encrypt",
	Long: `encrypt file`,
	PreRunE: func(cmd *cobra.Command, args []string) error {
		return setupEncryptArgs(cmd)
	},
	Run: func(cmd *cobra.Command, args []string) {
	},
}

func setupEncryptArgs(cmd *cobra.Command) error {
	return utils.Settings.BindPFlags(cmd.Flags())
}

func init() {
	rootCmd.AddCommand(encryptCMD)
	encryptCMD.PersistentFlags().StringP("inputfile", "i", "", "file path tobe encrypt")
	encryptCMD.PersistentFlags().StringP("outputfile", "o", "", "file path to output encrypted file")

	encryptCMD.AddCommand(encryptAESCMD)
	encryptAESCMD.Flags().StringP("secret", "s", "", "secret to encrypt file")
}

// encryptAESCMD encrypt file by aes
//
//   `go run cmd/main/main.go encrypt aes -i cmd/root.go -s 123`
var encryptAESCMD = &cobra.Command{
	Use:  "aes",
	Long: `encrypt file by aes`,
	PreRunE: func(cmd *cobra.Command, args []string) error {
		return setupEncryptAESArgs(cmd)
	},
	Run: func(cmd *cobra.Command, args []string) {
		encryptFileByAes()
	},
}

func setupEncryptAESArgs(cmd *cobra.Command) (err error) {
	if err = utils.Settings.BindPFlags(cmd.Flags()); err != nil {
		return err
	}

	if utils.Settings.GetString("inputfile") == "" {
		return fmt.Errorf("inputfile cannot be empty")
	}
	if utils.Settings.GetString("outputfile") == "" {
		out := utils.Settings.GetString("inputfile")
		ext := filepath.Ext(out)
		utils.Settings.Set("outputfile", strings.TrimSuffix(out, ext)+".enc"+ext)
	}
	if utils.Settings.GetString("secret") == "" {
		return fmt.Errorf("secret cannot be empty")
	}

	return nil
}

func encryptFileByAes() error {
	in := utils.Settings.GetString("inputfile")
	out := utils.Settings.GetString("outputfile")
	secret := []byte(utils.Settings.GetString("secret"))
	logger := utils.Logger.With(
		zap.String("in", in),
		zap.String("out", out),
	)
	logger.Info("encrypt file")

	cnt, err := ioutil.ReadFile(in)
	if err != nil {
		return errors.Wrapf(err, "read file `%s`", in)
	}

	cipher, err := utils.EncryptByAes(secret, cnt)
	if err != nil {
		return errors.Wrap(err, "encrypt")
	}

	if err = ioutil.WriteFile(out, cipher, os.ModePerm); err != nil {
		return errors.Wrapf(err, "write file `%s`", out)
	}

	logger.Info("successed")
	return nil
}
