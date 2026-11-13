//go:build linux

package transactionlocal

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"github.com/Laisky/errors/v2"
)

// temporaryName recognizes the decimal temporary names used by the supported
// toolchains. Recognition permits reopening crash leftovers, never their deletion.
func temporaryName(name, prefix string) bool {
	if !strings.HasPrefix(name, prefix) {
		return false
	}
	suffix := strings.TrimPrefix(name, prefix)
	if len(suffix) == 0 || len(suffix) > 20 {
		return false
	}
	for _, digit := range suffix {
		if digit < '0' || digit > '9' {
			return false
		}
	}
	return true
}

// generationName preserves the existing bounded single-component pointer syntax.
func generationName(name string) bool {
	return len(name) <= 255 && strings.HasPrefix(name, "generation-") &&
		filepath.Base(name) == name && !strings.ContainsAny(name, "/\\\x00\r\n")
}

// validateLayout rejects mixed roots without adopting arbitrary prefix matches.
// A validated current pointer explicitly identifies its active generation,
// including historical/imported names. Unreferenced recognized temporary entries
// are type/owner checked and preserved, because their names do not prove garbage.
func (engine *Engine) validateLayout() (result error) {
	lock, err := engine.acquireShared(context.Background())
	if err != nil {
		return err
	}
	defer func() { result = errors.Join(result, errors.Wrap(lock.Close(), "close layout validation lock")) }()

	current, err := engine.current()
	if err != nil {
		return err
	}
	active := ""
	if current != "" {
		active = filepath.Base(current)
	}
	entries, err := os.ReadDir(engine.root)
	if err != nil {
		return errors.Wrap(err, "read transaction layout")
	}
	// Validate every candidate before opening any pointer temporary.
	for _, entry := range entries {
		if !temporaryName(entry.Name(), "pointer-") {
			continue
		}
		info, err := os.Lstat(filepath.Join(engine.root, entry.Name()))
		if err != nil {
			return errors.Wrap(err, "stat retirement control entry")
		}
		if err = validateControlEntry(info, "retirement control entry", false); err != nil {
			return err
		}
	}
	proofs, err := engine.retirementProofs(entries)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		name := entry.Name()
		directory := false
		switch {
		case name == "current" || name == "lock":
		case active != "" && name == active:
			directory = true
		case temporaryName(name, "generation-") || temporaryName(name, "generation-empty-"):
			directory = true
		case temporaryName(name, "pointer-"):
		default:
			info, err := os.Lstat(filepath.Join(engine.root, name))
			if err != nil {
				return errors.Wrap(err, "stat possible retired generation")
			}
			if !info.IsDir() {
				return errors.New("transaction root contains unrelated entries; use a dedicated directory")
			}
			identity, err := directoryIdentity(info)
			if err != nil {
				return err
			}
			if _, proven := proofs[name][identity]; !proven {
				return errors.New("transaction root contains unrelated entries; use a dedicated directory")
			}
			directory = true
		}
		info, err := os.Lstat(filepath.Join(engine.root, name))
		if err != nil {
			return errors.Wrap(err, "stat transaction layout entry")
		}
		if err = validateControlEntry(info, "transaction layout entry", directory); err != nil {
			return err
		}
	}
	return nil
}

// collect retires only the previously published snapshot selected under the
// transaction lock. Unreferenced directories and pointer files are never swept
// by prefix, even when their names resemble valid temporary control entries.
func (engine *Engine) collect(previous, keep string) error {
	if previous == "" || previous == keep {
		return nil
	}
	if filepath.Dir(previous) != engine.root || filepath.Dir(keep) != engine.root {
		return errors.New("refuse snapshot retirement outside the transaction root")
	}
	if engine.retireSnapshot != nil {
		return engine.retireSnapshot(previous)
	}
	return removeOwnedSnapshot(previous)
}
