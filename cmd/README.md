some useful cmds.

Example:

append to your root cmd.

```go
import (
	gcmd "github.com/Laisky/go-utils/v6/cmd"
)

func init() {
	rootCmd.AddCommand(gcmd.EncryptCMD)
	rootCmd.AddCommand(gcmd.DecryptCMD)
}

```

## encrypt / decrypt aes

Secrets are never accepted as command-line arguments. `-s/--secret` has been removed and now fails with a
migration error. Choose one secret input:

- `--password-file <path|->`: password mode. The key is derived with Argon2id (t=3, m=64 MiB, p=4) under a fresh
  salt, and the output uses a versioned, self-describing format. Trailing CR/LF in the file are removed.
- `--key-file <path|->`: raw-key mode. The file holds a hex-encoded 16, 24 or 32-byte key; surrounding whitespace
  is ignored. The output uses the `crypto.AEADEncrypt` (AES-GCM) format.
- Neither flag: on a terminal, you are prompted for the password without echo (encryption asks twice). Without a
  terminal, the command fails.

On Unix, secret files must not be accessible by group or others (`chmod 600`). `-` reads the secret from stdin.

```sh
# file
go-utils encrypt aes -i config.toml --password-file ./pw.txt          # -> config.toml.enc
go-utils decrypt aes -i config.toml.enc --password-file ./pw.txt      # -> config.toml
pass show app/aes | go-utils encrypt aes -i config.toml --password-file -

# raw key
openssl rand -hex 32 > key.hex && chmod 600 key.hex
go-utils encrypt aes -i config.toml --key-file key.hex
go-utils decrypt aes -i config.toml.enc --key-file key.hex

# directory: encrypts each regular file directly inside the directory; password mode derives the key once per run
go-utils encrypt aes -i ./configs --password-file ./pw.txt
```

Outputs are written with mode 0600 through a temporary file and an atomic rename. An existing symlink (live or
dangling), directory or special file at the output path is refused and left untouched. An existing regular file
is replaced. Directory mode skips symlinks, special files and names that already end in `.enc`, and it enforces
file-count and byte budgets. Directory mode does not accept `-o`.

Files written by the removed `encrypt aes -s <password>` used the password bytes directly as the AES key. To
decrypt them, use `--legacy-password-as-key` together with a password input, then encrypt them again in password
mode:

```sh
go-utils decrypt aes -i old.enc -o old.txt --password-file ./pw.txt --legacy-password-as-key
go-utils encrypt aes -i old.txt --password-file ./pw.txt
```

See `crypto/aes_safety.md` for the formats and limits.
