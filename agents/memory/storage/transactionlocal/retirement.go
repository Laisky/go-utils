//go:build linux

package transactionlocal

import (
	"bytes"
	"encoding/json"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"syscall"

	"github.com/Laisky/errors/v2"
)

const (
	retirementFamily   = "go-utils/transactionlocal/retirement/"
	retirementHeader   = retirementFamily + "1\n"
	retirementSeal     = "\n--complete--\n"
	maxRetirementBytes = 2048
)

// snapshotIdentity binds ownership to a directory inode, rather than just a name.
type snapshotIdentity struct {
	Device uint64
	Inode  uint64
}

// retirementRecord proves that Previous was active before Next was published.
// It is private control metadata; project record and current-pointer formats do
// not change. It does not authorize automatic collection during recovery.
type retirementRecord struct {
	Previous []byte `json:"previous_bytes"`
	Next     []byte `json:"next_bytes"`
	Device   uint64 `json:"device"`
	Inode    uint64 `json:"inode"`
}

// directoryIdentity returns the Linux identity used to bind a retirement claim.
func directoryIdentity(info os.FileInfo) (snapshotIdentity, error) {
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return snapshotIdentity{}, errors.New("snapshot identity is unavailable")
	}
	return snapshotIdentity{Device: stat.Dev, Inode: stat.Ino}, nil
}

// prepareRetirement durably records the predecessor before publication can make
// its non-temporary name inactive. Failed/aborted receipts remain as safe
// leftovers, including when the next staging directory has already disappeared.
func (engine *Engine) prepareRetirement(previous, next string) (string, error) {
	if previous == "" {
		return "", nil
	}
	if filepath.Dir(previous) != engine.root || filepath.Dir(next) != engine.root ||
		!generationName(filepath.Base(previous)) || !generationName(filepath.Base(next)) || previous == next {
		return "", errors.New("retirement references must identify distinct root generations")
	}
	info, err := os.Lstat(previous)
	if err != nil {
		return "", errors.Wrap(err, "stat retirement predecessor")
	}
	if err = validateControlEntry(info, "retirement predecessor", true); err != nil {
		return "", err
	}
	identity, err := directoryIdentity(info)
	if err != nil {
		return "", err
	}
	record := retirementRecord{
		Previous: []byte(filepath.Base(previous)), Next: []byte(filepath.Base(next)),
		Device: identity.Device, Inode: identity.Inode,
	}
	body, err := json.Marshal(record)
	if err != nil {
		return "", errors.Wrap(err, "encode retirement ownership")
	}
	payload := retirementHeader + string(body) + retirementSeal
	if len(payload) > maxRetirementBytes {
		return "", errors.New("retirement ownership exceeds control budget")
	}
	file, err := os.CreateTemp(engine.root, "pointer-")
	if err != nil {
		return "", errors.Wrap(err, "create retirement ownership")
	}
	name := file.Name()
	written, writeErr := file.WriteString(payload)
	if writeErr == nil && written != len(payload) {
		writeErr = io.ErrShortWrite
	}
	syncErr := file.Sync()
	closeErr := file.Close()
	if err = errors.Join(
		errors.Wrap(writeErr, "write retirement ownership"),
		errors.Wrap(syncErr, "sync retirement ownership"),
		errors.Wrap(closeErr, "close retirement ownership"),
	); err != nil {
		return name, err
	}
	return name, errors.Wrap(syncDirectory(engine.root), "persist retirement ownership entry")
}

// finishRetirement persists predecessor removal before discarding the receipt.
// At every crash boundary a surviving predecessor still has durable ownership
// evidence. A resurrected receipt with a missing predecessor remains harmless.
func (engine *Engine) finishRetirement(previous, next, receipt string) error {
	if err := engine.collect(previous, next); err != nil {
		return err
	}
	if receipt == "" {
		return nil
	}
	if err := engine.syncRetirementDirectory(); err != nil {
		return errors.Wrap(err, "persist retired snapshot removal")
	}
	if err := removeIfPresent(receipt); err != nil {
		return err
	}
	return errors.Wrap(engine.syncRetirementDirectory(), "persist retirement receipt removal")
}

// readRetirement recognizes only complete sealed ownership records. Unsealed,
// truncated or ordinary legacy pointer temporaries prove nothing and are kept.
// Complete unsupported versions fail closed; present references are validated,
// while absent predecessors/successors are legitimate interrupted states.
func readRetirement(root *os.Root, name string) (record retirementRecord, recognized bool, result error) {
	file, err := root.Open(name)
	if err != nil {
		return retirementRecord{}, false, errors.Wrap(err, "open retirement candidate")
	}
	defer func() { result = errors.Join(result, errors.Wrap(file.Close(), "close retirement candidate")) }()
	info, err := file.Stat()
	if err != nil {
		return retirementRecord{}, false, errors.Wrap(err, "stat retirement candidate")
	}
	if err = validateControlEntry(info, "retirement candidate", false); err != nil {
		return retirementRecord{}, false, err
	}
	body, err := io.ReadAll(io.LimitReader(file, maxRetirementBytes+1))
	if err != nil {
		return retirementRecord{}, false, errors.Wrap(err, "read retirement candidate")
	}
	if len(body) > maxRetirementBytes {
		if bytes.HasPrefix(body, []byte(retirementFamily)) {
			return retirementRecord{}, false, errors.New("retirement ownership exceeds control budget")
		}
		return retirementRecord{}, false, nil
	}
	if !bytes.HasPrefix(body, []byte(retirementFamily)) || !bytes.HasSuffix(body, []byte(retirementSeal)) {
		return retirementRecord{}, false, nil
	}
	if !bytes.HasPrefix(body, []byte(retirementHeader)) {
		return retirementRecord{}, false, errors.New("unsupported sealed retirement version")
	}
	payload := body[len(retirementHeader) : len(body)-len(retirementSeal)]
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	var value retirementRecord
	if err = decoder.Decode(&value); err != nil {
		return retirementRecord{}, false, errors.Wrap(err, "decode sealed retirement ownership")
	}
	var extra any
	if err = decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err != nil {
			return retirementRecord{}, false, errors.Wrap(err, "decode retirement ownership tail")
		}
		return retirementRecord{}, false, errors.New("retirement ownership has trailing data")
	}
	previousName, nextName := string(value.Previous), string(value.Next)
	if !generationName(previousName) || !generationName(nextName) || previousName == nextName {
		return retirementRecord{}, false, errors.New("invalid retirement ownership references")
	}
	for _, reference := range []string{previousName, nextName} {
		info, err := root.Lstat(reference)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return retirementRecord{}, false, errors.Wrap(err, "stat retirement reference")
		}
		if err = validateControlEntry(info, "retirement reference", true); err != nil {
			return retirementRecord{}, false, err
		}
		if reference == previousName {
			identity, err := directoryIdentity(info)
			if err != nil {
				return retirementRecord{}, false, err
			}
			if identity != (snapshotIdentity{Device: value.Device, Inode: value.Inode}) {
				return retirementRecord{}, false, nil
			}
		}
	}
	return value, true, nil
}

// retirementProofs loads bounded records sequentially; memory use is linear in
// the dedicated root's entry count, as with layout validation itself.
func (engine *Engine) retirementProofs(
	entries []os.DirEntry,
) (proofs map[string]map[snapshotIdentity]struct{}, result error) {
	root, err := os.OpenRoot(engine.root)
	if err != nil {
		return nil, errors.Wrap(err, "open retirement namespace")
	}
	defer func() { result = errors.Join(result, errors.Wrap(root.Close(), "close retirement namespace")) }()
	proofs = make(map[string]map[snapshotIdentity]struct{})
	for _, entry := range entries {
		if !temporaryName(entry.Name(), "pointer-") {
			continue
		}
		record, recognized, err := readRetirement(root, entry.Name())
		if err != nil {
			return nil, err
		}
		if !recognized {
			continue
		}
		if proofs[string(record.Previous)] == nil {
			proofs[string(record.Previous)] = make(map[snapshotIdentity]struct{})
		}
		proofs[string(record.Previous)][snapshotIdentity{Device: record.Device, Inode: record.Inode}] = struct{}{}
	}
	return proofs, nil
}

// syncRetirementDirectory provides the real cleanup barrier in production and
// an internal deterministic filesystem-fault seam for phase-ordering tests.
func (engine *Engine) syncRetirementDirectory() error {
	if engine.retirementSync != nil {
		return engine.retirementSync(engine.root)
	}
	return syncDirectory(engine.root)
}
