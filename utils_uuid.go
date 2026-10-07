package utils

import (
	"github.com/Laisky/errors/v2"
	"github.com/google/uuid"
)

// UUID1 get uuid version 1
//
// Deprecated: use UUID7 instead
func UUID1() string {
	return uuid.Must(uuid.NewUUID()).String()
}

// UUID4 get uuid version 4
func UUID4() string {
	return uuid.Must(uuid.NewRandom()).String()
}

// UUID7 get uuid version 7
func UUID7() string {
	return uuid.Must(uuid.NewV7()).String()
}

// UUID7Bytes get uuid7 in bytes
func UUID7Bytes() uuid.UUID {
	return uuid.Must(uuid.NewV7())
}

// UUID7Itf general uuid7 interface
type UUID7Itf interface {
	// Timestamp get timestamp of uuid7
	Timestamp() uint64
	// String get string of uuid7
	String() string
	// Empty check if uuid7 is empty
	Empty() bool
}

type uuid7 struct {
	uuid.UUID
}

// Timestamp get timestamp of uuid7
func (u *uuid7) Timestamp() uint64 {
	sec, _ := u.UUID.Time().UnixTime()
	if sec < 0 {
		return 0
	}

	return uint64(sec)
}

// Empty check if uuid7 is empty
func (u *uuid7) Empty() bool {
	return u.UUID == uuid.Nil
}

// ParseUUID7 parse uuid7
func ParseUUID7(val string) (UUID7Itf, error) {
	u, err := uuid.Parse(val)
	if err != nil {
		return nil, errors.Wrapf(err, "parse uuid7 %q", val)
	}

	return &uuid7{UUID: u}, nil
}
