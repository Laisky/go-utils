package kms

import (
	"encoding/base64"
	"encoding/binary"
	"math"

	"github.com/Laisky/errors/v2"
)

// EncryptedDataVer version of encrypted data
type EncryptedDataVer uint8

const (
	// EncryptedItemVer1 encrypted item in ver1 layout
	//
	//  type EncryptedItem struct {
	//  	Version    EncryptedItemVer
	//  	KekID      uint16
	//  	DekID      []byte
	//  	Ciphertext []byte
	//  }
	//
	// layout:
	//
	//  - [0,1): version
	//  - [1,3): dek id length
	//  - [3,5): kek id
	//  - [5,5+len(dek id)): dek id
	//  - [5+len(dek id),5+len(dek id)+len(ciphertext)]: ciphertext
	EncryptedItemVer1 EncryptedDataVer = iota
)

// String name
func (e EncryptedDataVer) String() string {
	switch e {
	case EncryptedItemVer1:
		return "encrypted_item_ver_1"
	}

	return "encrypted_item_unimplemented"
}

// EncryptedData encrypted data
type EncryptedData struct {
	Version    EncryptedDataVer
	KekID      uint16
	DekID      []byte
	Ciphertext []byte
}

// Marshal marshal to bytes
func (e EncryptedData) Marshal() (data []byte, err error) {
	switch e.Version {
	case EncryptedItemVer1:
		dekIDLen := len(e.DekID)
		if dekIDLen > math.MaxUint16 {
			return nil, errors.Errorf("DekID length %d exceeds maximum allowed size of %d", dekIDLen, math.MaxUint16)
		}

		totalLen := 5 + dekIDLen + len(e.Ciphertext)
		if totalLen < 0 { // Check for integer overflow
			return nil, errors.Errorf("total length overflow")
		}

		data = make([]byte, totalLen)
		data[0] = byte(e.Version)
		binary.LittleEndian.PutUint16(data[1:3], uint16(dekIDLen))
		binary.LittleEndian.PutUint16(data[3:5], e.KekID)
		copy(data[5:5+dekIDLen], e.DekID)
		copy(data[5+dekIDLen:], e.Ciphertext)
	default:
		return nil, errors.Errorf("unknown version %q", e.Version.String())
	}

	return data, nil
}

// Unmarshal unmarshal from bytes
func (e *EncryptedData) Unmarshal(data []byte) error {
	if len(data) < 5 { // minimum length: version(1) + dekIDLen(2) + kekID(2)
		return errors.Errorf("data too short")
	}

	e.Version = EncryptedDataVer(data[0])
	switch e.Version {
	case EncryptedItemVer1:
		dekIDLen := int(binary.LittleEndian.Uint16(data[1:3]))
		totalLen := 5 + dekIDLen // 5 bytes header + dekID length
		if len(data) < totalLen {
			return errors.Errorf("data too short for DekID length %d", dekIDLen)
		}

		e.KekID = binary.LittleEndian.Uint16(data[3:5])

		// Handle DekID
		if dekIDLen > 0 {
			e.DekID = make([]byte, dekIDLen)
			copy(e.DekID, data[5:5+dekIDLen])
		} else {
			e.DekID = []byte{}
		}

		// Handle Ciphertext
		ciphertextLen := len(data) - totalLen
		if ciphertextLen > 0 {
			e.Ciphertext = make([]byte, ciphertextLen)
			copy(e.Ciphertext, data[totalLen:])
		} else {
			e.Ciphertext = []byte{}
		}

	default:
		return errors.Errorf("unknown version %q", e.Version.String())
	}

	return nil
}

// MarshalToString marshal to string
func (e EncryptedData) MarshalToString() (string, error) {
	data, err := e.Marshal()
	if err != nil {
		return "", errors.Wrap(err, "marshal")
	}

	return base64.StdEncoding.EncodeToString(data), nil
}

// UnmarshalFromString unmarshal from string
func (e *EncryptedData) UnmarshalFromString(s string) error {
	data, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		return errors.Wrap(err, "decode base64")
	}

	return e.Unmarshal(data)
}
