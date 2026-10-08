# Command-line utilities

`cmd/gutils` builds the `gutils` executable. Install it with:

```sh
go install github.com/Laisky/go-utils/v6/cmd/gutils@latest
```

This guide describes the current `v6` branch; a published version may lag it.
See the [root README](../README.md#command-line-toolbox) for branch builds and more commands.
Run `gutils --help` or `gutils <command> --help` to inspect available options.

## Embedding commands

Append exported Cobra commands to your application's root command:

```go
package main

import (
	gcmd "github.com/Laisky/go-utils/v6/cmd"
	"github.com/spf13/cobra"
)

// main builds a CLI containing the library's encryption and decryption commands.
func main() {
	root := &cobra.Command{Use: "my-tool"}
	root.AddCommand(gcmd.EncryptCMD)
	root.AddCommand(gcmd.DecryptCMD)
	if err := root.Execute(); err != nil {
		panic(err)
	}
}
```

## AES encryption and decryption

Secrets are never accepted as command-line arguments. `-s/--secret` has been removed and fails with a
migration error. Choose one secret input:

- `--password-file <path|->`: password mode. The key is derived with Argon2id (t=3, m=64 MiB, p=4) under a fresh
  salt, and the output uses a versioned, self-describing format. Trailing CR/LF in the file are removed.
- `--key-file <path|->`: raw-key mode. The file holds a hex-encoded 16, 24 or 32-byte key; surrounding whitespace
  is ignored. The output uses the `crypto.AEADEncrypt` (AES-GCM) format.
- Neither flag: on a terminal, you are prompted for the password without echo (encryption asks twice). Without a
  terminal, the command fails.

Provision secret files outside version control. On Unix they must not be accessible by group or others
(`chmod 600`). `-` reads the secret from stdin. Keep keys/passwords separate from encrypted data and back
up the material required for recovery.

```sh
# Password file already provisioned through your secret-management process.
chmod 600 ./pw.txt
gutils encrypt aes -i config.toml --password-file ./pw.txt          # -> config.toml.enc
gutils decrypt aes -i config.toml.enc -o config.restored.toml --password-file ./pw.txt
pass show app/aes | gutils encrypt aes -i config.toml --password-file -

# Raw key: use a private directory and an unused output filename.
# umask protects the key from its initial creation, before chmod.
umask 077
openssl rand -hex 32 > key.hex
chmod 600 key.hex
gutils encrypt aes -i config.toml --key-file key.hex
gutils decrypt aes -i config.toml.enc -o config.restored.toml --key-file key.hex

# Directory mode processes each regular file directly inside the directory.
gutils encrypt aes -i ./configs --password-file ./pw.txt
```

Password and raw-key commands above illustrate alternative modes. Keep track of which mode produced each
ciphertext; running them on the same input name replaces its existing regular `.enc` output. Never overwrite
the only key for existing ciphertext when generating a new key.

Outputs are written with mode 0600 through a temporary file and an atomic rename. An existing symlink (live or
dangling), directory or special file at the output path is refused and left untouched. **An existing regular file
is replaced.** Directory mode skips symlinks, special files and names that already end in `.enc`, and it enforces
file-count and byte budgets. It is not recursive and does not accept `-o`. Password mode derives the key once
per directory run. See the [output-path safety notes](output_safety.md) for caller responsibilities.

Files written by the removed `encrypt aes -s <password>` used the password bytes directly as the AES key. To
decrypt them, use `--legacy-password-as-key` together with a password input, then encrypt them again in password
mode:

```sh
gutils decrypt aes -i old.enc -o old.txt --password-file ./pw.txt --legacy-password-as-key
gutils encrypt aes -i old.txt --password-file ./pw.txt
```

See [AES formats and limits](../crypto/aes_safety.md) before migrating stored ciphertext.
