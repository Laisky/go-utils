// Package json implements encoding and decoding of JSON as defined in RFC 7159.
package json

import (
	"encoding/json"

	"github.com/Laisky/go-utils/v4/common"
	// json2 "github.com/go-json-experiment/json"
)

var (
	// Marshal marshal v to string
	Marshal = json.Marshal
	// MarshalIndent marshal v to string with indent
	MarshalIndent = json.MarshalIndent
	// NewDecoder returns a new decoder that reads from r.
	//
	// The decoder introduces its own buffering and may
	// read data from r beyond the JSON values requested.
	NewDecoder = json.NewDecoder
)

// MarshalToString marshal v to string
func MarshalToString(v interface{}) (string, error) {
	b, err := Marshal(v)
	return common.Bytes2Str(b), err
}
