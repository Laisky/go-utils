package kms

import (
	"encoding/binary"

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
		data = make([]byte, 5+len(e.DekID)+len(e.Ciphertext))
		dekIDLen := uint16(len(e.DekID))
		data[0] = byte(e.Version)
		binary.LittleEndian.PutUint16(data[1:3], dekIDLen)
		binary.LittleEndian.PutUint16(data[3:5], e.KekID)
		copy(data[5:5+len(e.DekID)], e.DekID)
		copy(data[5+len(e.DekID):], e.Ciphertext)
	default:
		return nil, errors.Errorf("unknown version %q", e.Version.String())
	}

	return data, nil
}

// Unmarshal unmarshal from bytes
func (e *EncryptedData) Unmarshal(data []byte) error {
	e.Version = EncryptedDataVer(data[0])
	switch e.Version {
	case EncryptedItemVer1:
		dekIDLen := binary.LittleEndian.Uint16(data[1:3])
		e.KekID = binary.LittleEndian.Uint16(data[3:5])
		e.DekID = data[5 : 5+dekIDLen]
		e.Ciphertext = data[5+dekIDLen:]
	default:
		return errors.Errorf("unknown version %q", e.Version.String())
	}

	return nil
}
