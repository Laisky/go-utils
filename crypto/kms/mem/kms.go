// Package mem is a multi-key KMS in pure memory
package mem

import (
	"context"
	"sync"

	"github.com/Laisky/errors/v2"
	"github.com/Laisky/zap"

	gcrypto "github.com/Laisky/go-utils/v4/crypto"
	gkms "github.com/Laisky/go-utils/v4/crypto/kms"
	glog "github.com/Laisky/go-utils/v4/log"
)

var (
	_ gkms.Interface = new(KMS)
)

// KMS insecure memory based KMS
//
// this KMS support multiple Keks,
// derieve dek by latest kek(keks[maxKeyID]).
type KMS struct {
	opt *kmsOption
	mu  sync.RWMutex
	// keks contain all keks
	//
	//  // map[kekID]kek
	//  map[uint16][]byte
	keks sync.Map

	maxKeyID uint16
}

type kmsOption struct {
	logger    glog.Logger
	aesKeyLen int
	dekIDLen  int
}

// KMSOption optional arguments for kms
type KMSOption func(*kmsOption) error

func (o *kmsOption) fillDefault() *kmsOption {
	o.aesKeyLen = 32
	o.dekIDLen = 128
	o.logger = glog.Shared.Named("kms")

	return o
}

func (o *kmsOption) applyOpts(opts ...KMSOption) (*kmsOption, error) {
	for i := range opts {
		if err := opts[i](o); err != nil {
			return nil, err
		}
	}

	return o, nil
}

// (optional) WithAesKeyLen set aes key length
//
// default to 32
func WithAesKeyLen(keyLen int) KMSOption {
	return func(o *kmsOption) error {
		o.aesKeyLen = keyLen
		return nil
	}
}

// WithDekKeyLen (optional) set aes key length
//
// default to 128
func WithDekKeyLen(keyLen int) KMSOption {
	return func(o *kmsOption) error {
		o.dekIDLen = keyLen
		return nil
	}
}

// WithLogger (optional) set internal logger
//
// default to gutils logger
func WithLogger(logger glog.Logger) KMSOption {
	return func(o *kmsOption) error {
		o.logger = logger
		return nil
	}
}

// New new kms
func New(keks map[uint16][]byte,
	opts ...KMSOption) (*KMS, error) {
	opt, err := new(kmsOption).fillDefault().applyOpts(opts...)
	if err != nil {
		return nil, errors.Wrap(err, "apply options")
	}

	kms := &KMS{
		opt: opt,
	}

	for i, k := range keks {
		if i >= kms.maxKeyID {
			kms.maxKeyID = i
		}

		storedKey := make([]byte, len(k))
		copy(storedKey, k)
		kms.keks.Store(i, storedKey)
	}

	return kms, nil
}

// AddKek add new kek
func (m *KMS) AddKek(ctx context.Context,
	kekID uint16,
	kek []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	storedKek := make([]byte, len(kek))
	copy(storedKek, kek)

	if _, loaded := m.keks.LoadOrStore(kekID, storedKek); loaded {
		return errors.Errorf("kek id already existed")
	}

	if kekID > m.maxKeyID {
		m.maxKeyID = kekID
	}

	return nil
}

// KEK return current used kek
func (m *KMS) Kek(ctx context.Context) (
	kekID uint16, kek []byte, err error) {
	m.mu.RLock()
	kekID = m.maxKeyID
	m.mu.RUnlock()

	v, ok := m.keks.Load(kekID)
	if !ok {
		m.opt.logger.Panic("cannot find maxkey id in keks",
			zap.Uint16("kek_id", kekID))
	}

	return kekID, v.([]byte), nil
}

// keks return all keks
func (m *KMS) Keks(ctx context.Context) (
	keks map[uint16][]byte, err error) {
	keks = make(map[uint16][]byte)
	m.keks.Range(func(key, value any) bool {
		keks[key.(uint16)] = value.([]byte)
		return true
	})

	return keks, nil
}

// DeriveKeyByID derive key by specific arguments
func (m *KMS) DeriveKeyByID(ctx context.Context,
	kekID uint16,
	dekID []byte,
	length int) (dek []byte, err error) {
	kek, ok := m.keks.Load(kekID)
	if !ok {
		return nil, errors.Errorf("kek %d not found", kekID)
	}

	return gcrypto.DeriveKeyByHKDF(kek.([]byte), dekID, length)
}

// DeriveKey derive random key
func (m *KMS) DeriveKey(ctx context.Context,
	length int) (kekID uint16, dekID, dek []byte, err error) {
	kekID, kek, err := m.Kek(ctx)
	if err != nil {
		return 0, nil, nil, errors.Wrap(err, "get current kek")
	}

	dekID, err = gcrypto.Salt(m.opt.dekIDLen)
	if err != nil {
		return 0, nil, nil, errors.Wrap(err, "generate dek id")
	}

	dek, err = gcrypto.DeriveKeyByHKDF(kek, dekID, length)
	if err != nil {
		return 0, nil, nil, errors.Wrap(err, "derive dek")
	}

	return kekID, dekID, dek, nil
}

// Encrypt encrypt by specific dek
func (m *KMS) EncryptByID(ctx context.Context,
	plaintext, additionalData []byte,
	kekID uint16,
	dekID []byte) (ciphertext []byte, err error) {
	dek, err := m.DeriveKeyByID(ctx, kekID, dekID, m.opt.aesKeyLen)
	if err != nil {
		return nil, errors.Wrap(err, "derive dek")
	}

	ciphertext, err = gcrypto.AEADEncrypt(dek, plaintext, additionalData)
	if err != nil {
		return nil, errors.Wrap(err, "encrypt by aead")
	}

	return ciphertext, nil
}

// Encrypt encrypt by random dek
func (m *KMS) Encrypt(ctx context.Context,
	plaintext, additionalData []byte) (ei gkms.EncryptedData, err error) {
	ei.Version = gkms.EncryptedItemVer1
	var dek []byte
	ei.KekID, ei.DekID, dek, err = m.DeriveKey(ctx, m.opt.aesKeyLen)
	if err != nil {
		return ei, errors.Wrap(err, "get current kek")
	}

	ei.Ciphertext, err = gcrypto.AEADEncrypt(dek, plaintext, additionalData)
	if err != nil {
		return ei, errors.Wrap(err, "encrypt by aead")
	}

	return ei, nil
}

// Decrypt decrypt ciphertext
func (m *KMS) Decrypt(ctx context.Context,
	ei gkms.EncryptedData,
	additionalData []byte) (plaintext []byte, err error) {
	dek, err := m.DeriveKeyByID(ctx, ei.KekID, ei.DekID, m.opt.aesKeyLen)
	if err != nil {
		return nil, errors.Wrap(err, "derive dek")
	}

	plaintext, err = gcrypto.AEADDecrypt(dek, ei.Ciphertext, additionalData)
	if err != nil {
		return nil, errors.Wrap(err, "decrypt by dek")
	}

	return plaintext, nil
}
