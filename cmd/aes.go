package cmd

// =====================================
// Encrypt / decrypt files by AES
//
// Secrets are never accepted on the command line. Password mode uses the
// versioned Argon2id + AES-256-GCM format (gcrypto.EncryptByPassword); raw-key
// mode uses the AES-GCM format of gcrypto.AEADEncrypt with a hex key file.
// =====================================

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/Laisky/errors/v2"
	"github.com/Laisky/zap"
	"github.com/spf13/cobra"

	gutils "github.com/Laisky/go-utils/v6"
	gcrypto "github.com/Laisky/go-utils/v6/crypto"
	"github.com/Laisky/go-utils/v6/log"
)

// aesEncryptedSuffix is appended to encrypted outputs by default.
const aesEncryptedSuffix = ".enc"

// errAESSecretFlagRemoved is returned when the removed -s/--secret flag is used.
var errAESSecretFlagRemoved = errors.New("-s/--secret has been removed: it exposed the secret in process " +
	"arguments and used the password bytes directly as the AES key. Use --password-file <path|-> " +
	"(Argon2id password mode), --key-file <path|-> (hex raw key), or run in a terminal to be prompted. " +
	"To decrypt files written with -s, use `decrypt aes --password-file <path|-> --legacy-password-as-key`")

// EncryptCMD encrypt files
var EncryptCMD = &cobra.Command{
	Use:   "encrypt",
	Short: "encrypt file or directory",
	Long: gutils.Dedent(`
		encrypt file or directory by aes

		Usage

			import (
				gcmd "github.com/Laisky/go-utils/v6/cmd"
			)

			func init() {
				rootCMD.AddCommand(gcmd.EncryptCMD)
				rootCMD.AddCommand(gcmd.DecryptCMD)
			}

		Run

			go run main.go encrypt aes -i <file_or_dir> --password-file <path|->
			go run main.go encrypt aes -i <file_or_dir> --key-file <path|->
			go run main.go encrypt aes -i <file_or_dir>   # prompts for a password on a terminal
	`),
	Args: NoExtraArgs,
}

// DecryptCMD decrypt files
var DecryptCMD = &cobra.Command{
	Use:   "decrypt",
	Short: "decrypt file",
	Long: gutils.Dedent(`
		decrypt a file encrypted by "encrypt"

		Run

			go run main.go decrypt aes -i <file>.enc --password-file <path|->
			go run main.go decrypt aes -i <file>.enc --key-file <path|->
			go run main.go decrypt aes -i <file>.enc --password-file <path|-> --legacy-password-as-key
	`),
	Args: NoExtraArgs,
}

// aesCLIArgs holds the flag values of one AES subcommand.
type aesCLIArgs struct {
	input, output         string
	passwordFile, keyFile string
	// legacySecret receives the removed -s/--secret flag so it can fail closed.
	legacySecret        string
	legacyPasswordAsKey bool
}

var (
	encryptAESArgs aesCLIArgs
	decryptAESArgs aesCLIArgs
	// aesCLIPasswordKDFParams are the Argon2id parameters used by `encrypt aes`.
	aesCLIPasswordKDFParams = gcrypto.DefaultPasswordKDFParams()
)

// init registers the encrypt/decrypt commands and their flags on the root command.
func init() {
	rootCmd.AddCommand(EncryptCMD)
	EncryptCMD.PersistentFlags().StringVarP(&encryptAESArgs.input,
		"input", "i", "", "file/directory path tobe encrypt")
	EncryptCMD.PersistentFlags().StringVarP(&encryptAESArgs.output,
		"output", "o", "",
		"file path to output encrypted file, default to <inputfilepath>.enc (not allowed for directories)")

	EncryptCMD.AddCommand(EncryptAESCMD)
	registerAESSecretFlags(EncryptAESCMD, &encryptAESArgs)
	EncryptAESCMD.Flags().StringVarP(&encryptAESArgs.legacySecret, "secret", "s", "",
		"REMOVED: secrets are never accepted as arguments; use --password-file or --key-file")

	rootCmd.AddCommand(DecryptCMD)
	DecryptCMD.PersistentFlags().StringVarP(&decryptAESArgs.input,
		"input", "i", "", "encrypted file path")
	DecryptCMD.PersistentFlags().StringVarP(&decryptAESArgs.output,
		"output", "o", "",
		"file path to output decrypted file, default to <input> without .enc (or <input>.dec)")

	DecryptCMD.AddCommand(DecryptAESCMD)
	registerAESSecretFlags(DecryptAESCMD, &decryptAESArgs)
	DecryptAESCMD.Flags().BoolVar(&decryptAESArgs.legacyPasswordAsKey, "legacy-password-as-key", false,
		"decrypt files written by the removed `encrypt aes -s`, which used the 16/24/32-byte password "+
			"directly as the AES key (requires a password input; insecure, migrate such files)")
}

// registerAESSecretFlags adds the --password-file and --key-file flags of c,
// bound to the fields of args.
func registerAESSecretFlags(c *cobra.Command, args *aesCLIArgs) {
	c.Flags().StringVar(&args.passwordFile, "password-file", "",
		"read the password from this file (`-` for stdin); trailing CR/LF are removed; "+
			"must not be group/world accessible")
	c.Flags().StringVar(&args.keyFile, "key-file", "",
		"read a hex-encoded 16/24/32-byte raw AES key from this file (`-` for stdin); "+
			"must not be group/world accessible")
}

// EncryptAESCMD encrypt files by aes
//
//	`go run cmd/main/main.go encrypt aes -i cmd/root.go --password-file ./pw.txt`
var EncryptAESCMD = &cobra.Command{
	Use:   "aes",
	Short: "encrypt a file or the files of a directory with AES-256-GCM",
	Long: gutils.Dedent(`
		encrypt a file, or every regular file directly inside a directory, by AES-GCM.

		Password mode (--password-file, or the interactive prompt) derives an AES-256 key
		with Argon2id (t=3, m=64 MiB, p=4) under a fresh random salt and writes a
		self-describing, versioned format. A directory run derives the key once.
		Raw-key mode (--key-file) uses a hex-encoded 16/24/32-byte key directly.

		Outputs are written with mode 0600 via a temporary file and an atomic rename;
		an existing symlink or special file at the output path is refused.
	`),
	Args: NoExtraArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		return runEncryptAES(cmd, &encryptAESArgs)
	},
}

// DecryptAESCMD decrypt files by aes
//
//	`go run cmd/main/main.go decrypt aes -i cmd/root.go.enc --password-file ./pw.txt`
var DecryptAESCMD = &cobra.Command{
	Use:   "aes",
	Short: "decrypt a file written by `encrypt aes`",
	Long: gutils.Dedent(`
		decrypt a file written by "encrypt aes".

		The password-based format is detected by its header. Raw-key files need
		--key-file. Files written by the removed "-s" flag need --password-file (or the
		prompt) together with --legacy-password-as-key.
	`),
	Args: NoExtraArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		return runDecryptAES(cmd, &decryptAESArgs)
	},
}

// commandContext returns cmd's context, or context.Background when unset.
func commandContext(cmd *cobra.Command) context.Context {
	if ctx := cmd.Context(); ctx != nil {
		return ctx
	}
	return context.Background()
}

// runEncryptAES validates args, reads the secret without using argv, and
// encrypts the input file or directory. It returns the first error.
func runEncryptAES(cmd *cobra.Command, args *aesCLIArgs) error {
	if args.legacySecret != "" || cmd.Flags().Changed("secret") {
		return errors.WithStack(errAESSecretFlagRemoved)
	}
	if args.input == "" {
		return errors.New("--input cannot be empty")
	}

	st, err := os.Stat(args.input)
	if err != nil {
		return errors.Wrap(err, "read input path")
	}
	if st.IsDir() && args.output != "" {
		return errors.New("--output is not supported for a directory input; outputs are written next to each file")
	}

	secret, err := readAESCLISecret(cmd, args.passwordFile, args.keyFile, true)
	if err != nil {
		return errors.WithStack(err)
	}
	defer secret.wipe()

	if st.IsDir() {
		log.Shared.Info("encrypt files in dir", zap.String("path", args.input), zap.String("mode", secret.mode()))
		return encryptDirByAESCLI(commandContext(cmd), args.input, secret)
	}

	output := args.output
	if output == "" {
		output = args.input + aesEncryptedSuffix
	}
	return encryptFileByAESCLI(args.input, output, secret)
}

// encryptDirByAESCLI encrypts the files of dir in password or raw-key mode.
// Password mode derives the key once for the whole run.
func encryptDirByAESCLI(ctx context.Context, dir string, secret *aesCLISecret) error {
	if secret.key != nil {
		return errors.WithStack(gcrypto.AESEncryptFilesInDirWithContext(ctx, dir, secret.key))
	}

	enc, err := gcrypto.NewPasswordEncryptor(secret.password, gcrypto.WithPasswordKDFParams(aesCLIPasswordKDFParams))
	if err != nil {
		return errors.Wrap(err, "derive password key")
	}

	return errors.WithStack(gcrypto.PasswordEncryptFilesInDirWithContext(ctx, dir, enc))
}

// encryptFileByAESCLI encrypts the file in and safely publishes the result at out.
func encryptFileByAESCLI(in, out string, secret *aesCLISecret) error {
	logger := log.Shared.With(zap.String("in", in), zap.String("out", out), zap.String("mode", secret.mode()))
	logger.Info("encrypt file")

	plaintext, err := os.ReadFile(in)
	if err != nil {
		return errors.Wrapf(err, "read file `%s`", in)
	}
	defer clear(plaintext)

	var ciphertext []byte
	if secret.key != nil {
		ciphertext, err = gcrypto.AEADEncrypt(secret.key, plaintext, nil)
	} else {
		ciphertext, err = gcrypto.EncryptByPassword(secret.password, plaintext,
			gcrypto.WithPasswordKDFParams(aesCLIPasswordKDFParams))
	}
	if err != nil {
		return errors.Wrap(err, "encrypt")
	}

	if err = writeAESCLIOutput(out, ciphertext); err != nil {
		return errors.WithStack(err)
	}

	logger.Info("succeeded")
	return nil
}

// runDecryptAES validates args, reads the secret without using argv, decrypts
// the input file and safely publishes the plaintext. It returns the first error.
func runDecryptAES(cmd *cobra.Command, args *aesCLIArgs) error {
	if args.input == "" {
		return errors.New("--input cannot be empty")
	}
	if args.legacyPasswordAsKey && args.keyFile != "" {
		return errors.New("--legacy-password-as-key needs a password input, not --key-file")
	}

	data, err := os.ReadFile(args.input)
	if err != nil {
		return errors.Wrapf(err, "read file `%s`", args.input)
	}

	output := args.output
	if output == "" {
		output = defaultAESDecryptOutput(args.input)
	}

	secret, err := readAESCLISecret(cmd, args.passwordFile, args.keyFile, false)
	if err != nil {
		return errors.WithStack(err)
	}
	defer secret.wipe()

	plaintext, err := decryptAESCLIData(data, secret, args.legacyPasswordAsKey)
	if err != nil {
		return errors.WithStack(err)
	}
	defer clear(plaintext)

	if err = writeAESCLIOutput(output, plaintext); err != nil {
		return errors.WithStack(err)
	}

	log.Shared.Info("decrypt file", zap.String("in", args.input), zap.String("out", output),
		zap.String("mode", secret.mode()), zap.Bool("legacy", args.legacyPasswordAsKey))
	return nil
}

// defaultAESDecryptOutput strips the .enc suffix from in, or appends .dec when
// there is no such suffix.
func defaultAESDecryptOutput(in string) string {
	trimmed, ok := strings.CutSuffix(in, aesEncryptedSuffix)
	if ok && trimmed != "" && !os.IsPathSeparator(trimmed[len(trimmed)-1]) {
		return trimmed
	}
	return in + ".dec"
}

// decryptAESCLIData decrypts data in raw-key, legacy or password mode and
// returns the plaintext, or an error with a migration hint.
func decryptAESCLIData(data []byte, secret *aesCLISecret, legacy bool) ([]byte, error) {
	switch {
	case secret.key != nil:
		plaintext, err := gcrypto.AEADDecrypt(secret.key, data, nil)
		if err != nil && gcrypto.IsPasswordEncrypted(data) {
			return nil, errors.Wrap(err, "decrypt with raw key (input looks password-encrypted; use --password-file)")
		}
		return plaintext, errors.Wrap(err, "decrypt with raw key")
	case legacy:
		switch len(secret.password) {
		case 16, 24, 32:
		default:
			return nil, errors.Errorf("--legacy-password-as-key needs a 16, 24 or 32-byte password, got %d bytes",
				len(secret.password))
		}
		plaintext, err := gcrypto.AEADDecrypt(secret.password, data, nil)
		return plaintext, errors.Wrap(err, "decrypt legacy password-as-key file")
	case gcrypto.IsPasswordEncrypted(data):
		plaintext, err := gcrypto.DecryptByPassword(secret.password, data)
		return plaintext, errors.WithStack(err)
	default:
		return nil, errors.New("input is not in the password-based format: files written by the removed " +
			"`encrypt aes -s` need --legacy-password-as-key, raw-key files need --key-file")
	}
}

// writeAESCLIOutput publishes content at path with mode 0600 through a private
// temporary file and an atomic rename. An existing path that is not a regular
// file (a symlink, even a dangling one, a directory or a special file) is
// refused and left untouched; a regular file is replaced.
func writeAESCLIOutput(path string, content []byte) error {
	path = filepath.Clean(path)
	st, err := os.Lstat(path)
	switch {
	case err == nil && !st.Mode().IsRegular():
		return errors.Errorf("refusing to write `%s`: it exists and is not a regular file "+
			"(symlink, directory or special file); remove it or choose another --output", path)
	case err != nil && !errors.Is(err, fs.ErrNotExist):
		return errors.Wrapf(err, "inspect output `%s`", path)
	}

	if err := gutils.ReplaceFile(path, content, 0o600); err != nil {
		return errors.Wrapf(err, "write file `%s`", path)
	}

	return nil
}
