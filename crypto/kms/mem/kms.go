// Package mem is a multi-key KMS in pure memory
package mem

import (
	"context"
	"sync"

	"github.com/Laisky/errors/v2"
	"github.com/Laisky/zap"

	gcrypto "github.com/Laisky/go-utils/v4/crypto"
	glog "github.com/Laisky/go-utils/v4/log"
)

// Interface interface of kms
type Interface interface {
	AddNewMasterKey(ctx context.Context, masterKeyID uint32, masterKey []byte) error
	MasterKey(ctx context.Context) (masterKeyID uint32, masterKey []byte, err error)
	MasterKeys(ctx context.Context) (masterKeys map[uint32][]byte, err error)
	DeriveKeyByID(ctx context.Context,
		masterKeyID uint32,
		dekID []byte,
		length int) (dek []byte, err error)
	DeriveKey(ctx context.Context, length int) (masterKeyID uint32, dekID, dek []byte, err error)
	Encrypt(ctx context.Context, plaintext,
		additionalData []byte) (masterKeyID uint32, dekID, ciphertext []byte, err error)
	Decrypt(ctx context.Context,
		masterKeyID uint32,
		dekID, ciphertext, additionalData []byte) (plaintext []byte, err error)
}

// KMS insecure memory based KMS
//
// this KMS support multiple master keys.
// derieve DEK by latest masterkey(masterKeys[maxKeyID]).
type KMS struct {
	opt *kmsOption
	mu  sync.RWMutex
	// masterKeys contain all master keys
	//
	//  // map[masterKeyID]masterkey
	//  map[uint32][]byte
	masterKeys sync.Map

	maxKeyID uint32
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
func New(masterKeys map[uint32][]byte,
	opts ...KMSOption) (*KMS, error) {
	opt, err := new(kmsOption).fillDefault().applyOpts(opts...)
	if err != nil {
		return nil, errors.Wrap(err, "apply options")
	}

	kms := &KMS{
		opt: opt,
	}

	for i, k := range masterKeys {
		if i >= kms.maxKeyID {
			kms.maxKeyID = i
		}

		storedKey := make([]byte, len(k))
		copy(storedKey, k)
		kms.masterKeys.Store(i, storedKey)
	}

	return kms, nil
}

// AddNewMasterKey add new master key
func (m *KMS) AddNewMasterKey(ctx context.Context,
	masterKeyID uint32,
	masterKey []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	storedMasterKey := make([]byte, len(masterKey))
	copy(storedMasterKey, masterKey)

	if _, loaded := m.masterKeys.LoadOrStore(masterKeyID, storedMasterKey); loaded {
		return errors.Errorf("masterkey id already existed")
	}

	if masterKeyID > m.maxKeyID {
		m.maxKeyID = masterKeyID
	}

	return nil
}

// MasterKey return current used master key
func (m *KMS) MasterKey(ctx context.Context) (
	masterKeyID uint32, masterKey []byte, err error) {
	m.mu.RLock()
	masterKeyID = m.maxKeyID
	m.mu.RUnlock()

	v, ok := m.masterKeys.Load(masterKeyID)
	if !ok {
		m.opt.logger.Panic("cannot find maxkey id in master keys",
			zap.Uint32("master_key_id", masterKeyID))
	}

	return masterKeyID, v.([]byte), nil
}

// MasterKeys return all master keys
func (m *KMS) MasterKeys(ctx context.Context) (
	masterKeys map[uint32][]byte, err error) {
	masterKeys = make(map[uint32][]byte)
	m.masterKeys.Range(func(key, value any) bool {
		masterKeys[key.(uint32)] = value.([]byte)
		return true
	})

	return masterKeys, nil
}

// DeriveKeyByID derive key by specific arguments
func (m *KMS) DeriveKeyByID(ctx context.Context,
	masterKeyID uint32,
	dekID []byte,
	length int) (dek []byte, err error) {
	masterkey, ok := m.masterKeys.Load(masterKeyID)
	if !ok {
		return nil, errors.Errorf("masterkey %d not found", masterKeyID)
	}

	return gcrypto.DeriveKeyByHKDF(masterkey.([]byte), dekID, length)
}

// DeriveKey derive random key
func (m *KMS) DeriveKey(ctx context.Context,
	length int) (masterKeyID uint32, dekID, dek []byte, err error) {
	masterKeyID, masterkey, err := m.MasterKey(ctx)
	if err != nil {
		return 0, nil, nil, errors.Wrap(err, "get current master key")
	}

	dekID, err = gcrypto.Salt(m.opt.dekIDLen)
	if err != nil {
		return 0, nil, nil, errors.Wrap(err, "generate dek id")
	}

	dek, err = gcrypto.DeriveKeyByHKDF(masterkey, dekID, length)
	if err != nil {
		return 0, nil, nil, errors.Wrap(err, "derive dek")
	}

	return masterKeyID, dekID, dek, nil
}

// Encrypt encrypt by specific dek
func (m *KMS) EncryptByID(ctx context.Context,
	plaintext, additionalData []byte,
	masterKeyID uint32,
	dekID []byte) (ciphertext []byte, err error) {
	dek, err := m.DeriveKeyByID(ctx, masterKeyID, dekID, m.opt.aesKeyLen)
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
	plaintext, additionalData []byte) (masterKeyID uint32, dekID, ciphertext []byte, err error) {
	masterKeyID, dekID, dek, err := m.DeriveKey(ctx, m.opt.aesKeyLen)
	if err != nil {
		return 0, nil, nil, errors.Wrap(err, "get current master key")
	}

	ciphertext, err = gcrypto.AEADEncrypt(dek, plaintext, additionalData)
	if err != nil {
		return 0, nil, nil, errors.Wrap(err, "encrypt by aead")
	}

	return masterKeyID, dekID, ciphertext, nil
}

// Decrypt decrypt ciphertext
func (m *KMS) Decrypt(ctx context.Context,
	masterKeyID uint32,
	dekID, ciphertext, additionalData []byte) (plaintext []byte, err error) {
	dek, err := m.DeriveKeyByID(ctx, masterKeyID, dekID, m.opt.aesKeyLen)
	if err != nil {
		return nil, errors.Wrap(err, "derive dek")
	}

	plaintext, err = gcrypto.AEADDecrypt(dek, ciphertext, additionalData)
	if err != nil {
		return nil, errors.Wrap(err, "decrypt by dek")
	}

	return plaintext, nil
}
