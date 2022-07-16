package cmd

// =====================================
// Encrypt File
//
// 1. encrypt file by aes
// =====================================

import (
	"io/ioutil"
	"os"

	gutils "github.com/Laisky/go-utils/v2"
	"github.com/Laisky/go-utils/v2/encrypt"
	"github.com/Laisky/go-utils/v2/log"
	"github.com/Laisky/zap"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
)

// EncryptCMD encrypt files
var EncryptCMD = &cobra.Command{
	Use:   "encrypt",
	Short: "encrypt file or directory",
	Long: gutils.Dedent(`
		encrypt file or directory by aes

		Usage

			import (
				gcmd "github.com/Laisky/go-utils/v2/cmd"
			)

			func init() {
				rootCMD.AddCommand(gcmd.EncryptCMD)
			}

		Run

			go run -race main.go encrypt aes -i <file_path> -s <password>
	`),
	Args: NoExtraArgs,
}

var (
	inputpath, outputpath, secret string
)

func init() {
	rootCmd.AddCommand(EncryptCMD)
	EncryptCMD.PersistentFlags().StringVarP(&inputpath,
		"input", "i", "", "file/directory path tobe encrypt")
	EncryptCMD.PersistentFlags().StringVarP(&outputpath,
		"output", "o", "",
		"file/directory path to output encrypted file, default to <inputfilepath>.enc")

	EncryptCMD.AddCommand(EncryptAESCMD)
	EncryptAESCMD.Flags().StringVarP(&secret, "secret", "s", "", "secret to encrypt file")
}

// EncryptAESCMD encrypt files by aes
//
//   `go run cmd/main/main.go encrypt aes -i cmd/root.go -s 123`
var EncryptAESCMD = &cobra.Command{
	Use:   "aes",
	Short: "encrypt file by aes, key's length must be 16/24/32",
	Long:  `encrypt file by aes`,
	PreRunE: func(cmd *cobra.Command, args []string) error {
		return setupEncryptAESArgs(cmd)
	},
	Run: func(cmd *cobra.Command, args []string) {
		fs, err := os.Stat(inputpath)
		if err != nil {
			log.Shared.Panic("read path", zap.Error(err))
		}

		if fs.IsDir() {
			if err = encryptDirFileByAes(); err != nil {
				log.Shared.Panic("encrypt files in dir", zap.Error(err))
			}
		} else {
			if err = encryptFileByAes(); err != nil {
				log.Shared.Panic("encrypt file", zap.Error(err))
			}
		}
	},
}

func setupEncryptAESArgs(cmd *cobra.Command) (err error) {
	if inputpath == "" {
		return errors.Errorf("inputfile cannot be empty")
	}
	if secret == "" {
		return errors.Errorf("secret cannot be empty")
	}

	if outputpath == "" {
		outputpath = inputpath + ".enc"
	}

	return nil
}

func encryptDirFileByAes() error {
	secret := []byte(secret)
	log.Shared.Info("encrypt files in dir", zap.String("path", inputpath))

	return encrypt.AESEncryptFilesInDir(inputpath, secret)
}

func encryptFileByAes() error {
	in := inputpath
	out := outputpath
	secret := []byte(secret)
	logger := log.Shared.With(
		zap.String("in", in),
		zap.String("out", out),
	)
	logger.Info("encrypt file")

	cnt, err := ioutil.ReadFile(in)
	if err != nil {
		return errors.Wrapf(err, "read file `%s`", in)
	}

	cipher, err := encrypt.AesEncrypt(secret, cnt)
	if err != nil {
		return errors.Wrap(err, "encrypt")
	}

	if err = ioutil.WriteFile(out, cipher, os.ModePerm); err != nil {
		return errors.Wrapf(err, "write file `%s`", out)
	}

	logger.Info("successed")
	return nil
}
