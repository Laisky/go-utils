package kms

import "context"

// Interface interface of kms
type Interface interface {
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
		additionalData []byte) (ed EncryptedData, err error)
	// Decrypt decrypt data
	Decrypt(ctx context.Context,
		ed EncryptedData, additionalData []byte) (plaintext []byte, err error)
}
