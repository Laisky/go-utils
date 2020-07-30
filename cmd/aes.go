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

// EncryptCMD encrypt files
var EncryptCMD = &cobra.Command{
	Use:  "encrypt",
	Long: `encrypt file`,
	Args: NoExtraArgs,
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
	rootCmd.AddCommand(EncryptCMD)
	EncryptCMD.PersistentFlags().StringP("inputfile", "i", "", "file path tobe encrypt")
	EncryptCMD.PersistentFlags().StringP("outputfile", "o", "", "file path to output encrypted file")

	EncryptCMD.AddCommand(EncryptAESCMD)
	EncryptAESCMD.Flags().StringP("secret", "s", "", "secret to encrypt file")
}

// EncryptAESCMD encrypt files by aes
//
//   `go run cmd/main/main.go encrypt aes -i cmd/root.go -s 123`
var EncryptAESCMD = &cobra.Command{
	Use:  "aes",
	Long: `encrypt file by aes`,
	PreRunE: func(cmd *cobra.Command, args []string) error {
		return setupEncryptAESArgs(cmd)
	},
	Run: func(cmd *cobra.Command, args []string) {
		fs, err := os.Stat(utils.Settings.GetString("inputfile"))
		if err != nil {
			utils.Logger.Panic("read path", zap.Error(err))
		}

		if fs.IsDir() {
			if err = encryptDirFileByAes(); err != nil {
				utils.Logger.Panic("encrypt files in dir", zap.Error(err))
			}
		} else {
			if err = encryptFileByAes(); err != nil {
				utils.Logger.Panic("encrypt file", zap.Error(err))
			}
		}
	},
}

func setupEncryptAESArgs(cmd *cobra.Command) (err error) {
	if err = utils.Settings.BindPFlags(cmd.Flags()); err != nil {
		return err
	}

	if utils.Settings.GetString("inputfile") == "" &&
		utils.Settings.GetString("inputdir") == "" {
		return fmt.Errorf("inputfile & inputdir cannot both be empty")
	}

	if utils.Settings.GetString("outputfile") == "" &&
		utils.Settings.GetString("inputfile") != "" {
		out := utils.Settings.GetString("inputfile")
		ext := filepath.Ext(out)
		utils.Settings.Set("outputfile", strings.TrimSuffix(out, ext)+".enc"+ext)
	}

	if utils.Settings.GetString("outputdir") == "" {
		utils.Settings.Set("outputdir", utils.Settings.GetString("inputdir"))
	}

	if utils.Settings.GetString("secret") == "" {
		return fmt.Errorf("secret cannot be empty")
	}

	return nil
}

func encryptDirFileByAes() error {
	in := utils.Settings.GetString("inputfile")
	out := utils.Settings.GetString("outputfile")
	secret := []byte(utils.Settings.GetString("secret"))
	logger := utils.Logger.With(
		zap.String("in", in),
		zap.String("out", out),
	)
	logger.Info("encrypt files in dir")

	return utils.AESEncryptFilesInDir(in, secret)
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
