// Package kms provides a simple kms interface.
package kms

import (
	"context"
	"fmt"
)

// Interface interface of kms
type Interface interface {
	// Status get current status
	Status() Status
	// AddKek add new kek
	AddKek(ctx context.Context, kekID uint16, kek []byte) error
	// Kek get current used kek
	Kek(ctx context.Context) (kekID uint16, kek []byte, err error)
	// Keks export all keks
	Keks(ctx context.Context) (keks map[uint16][]byte, err error)
	// DeriveKeyByID derive key by specific kek id  and dek id
	DeriveKeyByID(ctx context.Context,
		kekID uint16,
		dekID []byte,
		length int) (dek []byte, err error)
	// DeriveKey derive random key by current kek
	DeriveKey(ctx context.Context, length int) (kekID uint16, dekID, dek []byte, err error)
	// Encrypt encrypt data
	Encrypt(ctx context.Context, plaintext,
		additionalData []byte) (ed *EncryptedData, err error)
	// Decrypt decrypt data
	Decrypt(ctx context.Context,
		ed *EncryptedData, additionalData []byte) (plaintext []byte, err error)
}

// Status status of kms
type Status uint32

// String return string of status
func (s Status) String() string {
	switch s {
	case StatusImplemented:
		return "implemented"
	case StatusNoKeK:
		return "no_kek"
	case StatusReady:
		return "ready"
	}

	return fmt.Sprintf("unknown status %d", s)
}

const (
	// StatusImplemented status implemented
	StatusImplemented Status = iota
	// StatusNoKeK need call `AddKek` add at least one kek
	StatusNoKeK
	// StatusReady status ok
	StatusReady
)
