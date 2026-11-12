package memory

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"

	"github.com/Laisky/errors/v2"

	storageengine "github.com/Laisky/go-utils/v6/agents/memory/storage"
)

// ArchiveFormat identifies an explicit archive representation, not an automatic
// decoder fallback. It affects only newly written archives, never existing files.
type ArchiveFormat string

const (
	// ArchiveFormatZstd preserves historical binary .zst files and is the default.
	// The backend must preserve arbitrary bytes through both Write and Read.
	ArchiveFormatZstd ArchiveFormat = ""
	// ArchiveFormatZstdBase64V1 writes a UTF-8 JSON envelope with a base64 zstd
	// frame to a distinct .zst.b64.json file. Old zstd readers cannot consume it.
	ArchiveFormatZstdBase64V1 ArchiveFormat = "zstd-base64-v1"
)

// archiveEnvelopeV1 carries a standard zstd frame through text-only storage.
type archiveEnvelopeV1 struct {
	Format ArchiveFormat `json:"format"`
	Data   string        `json:"data"`
}

// validateArchiveFormat rejects unknown archive formats without writing data.
func validateArchiveFormat(format ArchiveFormat) error {
	switch format {
	case ArchiveFormatZstd, ArchiveFormatZstdBase64V1:
		return nil
	default:
		return errors.Errorf("unsupported archive format %q", format)
	}
}

// encodeArchive encodes a zstd frame in the explicitly selected storage format.
func encodeArchive(format ArchiveFormat, frame []byte) (string, error) {
	if err := validateArchiveFormat(format); err != nil {
		return "", errors.Wrap(err, "validate archive format")
	}
	if format == ArchiveFormatZstd {
		return string(frame), nil
	}
	body, err := json.Marshal(archiveEnvelopeV1{
		Format: format, Data: base64.StdEncoding.EncodeToString(frame),
	})
	if err != nil {
		return "", errors.Wrap(err, "marshal archive envelope")
	}
	return string(body), nil
}

// DecodeArchive returns the standard compressed zstd frame from content in the
// explicitly selected format. It does not decompress, authenticate or infer a
// format. Callers must separately bound decompression and validate the raw data.
// Choose ArchiveFormatZstd for historical .zst files, or
// ArchiveFormatZstdBase64V1 for .zst.b64.json files. Invalid envelopes return an
// error without trying a legacy decoder. Existing binary archives are unchanged.
func DecodeArchive(format ArchiveFormat, content string) ([]byte, error) {
	if err := validateArchiveFormat(format); err != nil {
		return nil, errors.Wrap(err, "validate archive format")
	}
	if format == ArchiveFormatZstd {
		return []byte(content), nil
	}
	var envelope archiveEnvelopeV1
	if err := json.Unmarshal([]byte(content), &envelope); err != nil {
		return nil, errors.Wrap(err, "decode archive envelope")
	}
	if envelope.Format != ArchiveFormatZstdBase64V1 || envelope.Data == "" {
		return nil, errors.New("invalid archive envelope format or data")
	}
	frame, err := base64.StdEncoding.Strict().DecodeString(envelope.Data)
	if err != nil {
		return nil, errors.Wrap(err, "decode archive frame")
	}
	return frame, nil
}

// writeVerifiedArchive persists frame and verifies byte-for-byte readback before
// allowing the caller to remove its raw source. This proves the exact output of
// compressZstd survived storage, including a real JSON/UTF-8 transport boundary.
func (engine *StandardEngine) writeVerifiedArchive(
	ctx context.Context, project, archivePath string, frame []byte,
) error {
	body, err := encodeArchive(engine.conf.ArchiveFormat, frame)
	if err != nil {
		return errors.Wrap(err, "encode archive")
	}
	if err = engine.storage.Write(ctx, project, archivePath, body, storageengine.WriteModeTruncate, 0); err != nil {
		return errors.Wrap(err, "write archive")
	}
	stored, err := engine.storage.Read(ctx, project, archivePath, 0, -1)
	if err != nil {
		return errors.Wrap(err, "read archive for verification; raw shard retained")
	}
	persistedFrame, err := DecodeArchive(engine.conf.ArchiveFormat, stored)
	if err != nil {
		return errors.Wrap(err, "decode archive for verification; raw shard retained")
	}
	if !bytes.Equal(frame, persistedFrame) {
		return errors.New("archive readback differs from compressed source; raw shard retained; " +
			"text-only storage requires ArchiveFormatZstdBase64V1")
	}
	return nil
}
