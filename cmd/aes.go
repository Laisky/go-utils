package cmd

// =====================================
// Encrypt File
//
// 1. encrypt file by aes
// =====================================

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	gutils "github.com/Laisky/go-utils"
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
	return gutils.Settings.BindPFlags(cmd.Flags())
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
		fs, err := os.Stat(gutils.Settings.GetString("inputfile"))
		if err != nil {
			gutils.Logger.Panic("read path", zap.Error(err))
		}

		if fs.IsDir() {
			if err = encryptDirFileByAes(); err != nil {
				gutils.Logger.Panic("encrypt files in dir", zap.Error(err))
			}
		} else {
			if err = encryptFileByAes(); err != nil {
				gutils.Logger.Panic("encrypt file", zap.Error(err))
			}
		}
	},
}

func setupEncryptAESArgs(cmd *cobra.Command) (err error) {
	if err = gutils.Settings.BindPFlags(cmd.Flags()); err != nil {
		return err
	}

	if gutils.Settings.GetString("inputfile") == "" &&
		gutils.Settings.GetString("inputdir") == "" {
		return errors.Errorf("inputfile & inputdir cannot both be empty")
	}

	if gutils.Settings.GetString("outputfile") == "" &&
		gutils.Settings.GetString("inputfile") != "" {
		out := gutils.Settings.GetString("inputfile")
		ext := filepath.Ext(out)
		gutils.Settings.Set("outputfile", strings.TrimSuffix(out, ext)+".enc"+ext)
	}

	if gutils.Settings.GetString("outputdir") == "" {
		gutils.Settings.Set("outputdir", gutils.Settings.GetString("inputdir"))
	}

	if gutils.Settings.GetString("secret") == "" {
		return errors.Errorf("secret cannot be empty")
	}

	return nil
}

func encryptDirFileByAes() error {
	in := gutils.Settings.GetString("inputfile")
	out := gutils.Settings.GetString("outputfile")
	secret := []byte(gutils.Settings.GetString("secret"))
	logger := gutils.Logger.With(
		zap.String("in", in),
		zap.String("out", out),
	)
	logger.Info("encrypt files in dir")

	return gutils.AESEncryptFilesInDir(in, secret)
}

func encryptFileByAes() error {
	in := gutils.Settings.GetString("inputfile")
	out := gutils.Settings.GetString("outputfile")
	secret := []byte(gutils.Settings.GetString("secret"))
	logger := gutils.Logger.With(
		zap.String("in", in),
		zap.String("out", out),
	)
	logger.Info("encrypt file")

	cnt, err := ioutil.ReadFile(in)
	if err != nil {
		return errors.Wrapf(err, "read file `%s`", in)
	}

	cipher, err := gutils.EncryptByAes(secret, cnt)
	if err != nil {
		return errors.Wrap(err, "encrypt")
	}

	if err = ioutil.WriteFile(out, cipher, os.ModePerm); err != nil {
		return errors.Wrapf(err, "write file `%s`", out)
	}

	logger.Info("successed")
	return nil
}
