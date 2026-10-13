package utils

import (
	"bytes"
	"encoding/json"
	"io"

	"github.com/Laisky/errors/v2"
)

// validateRBACPolicyJSON rejects duplicate object fields and excessive syntactic
// depth before interpretation, so different decoders cannot disagree on grants.
func validateRBACPolicyJSON(data []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	if err := readRBACPolicyJSONValue(decoder, 0); err != nil {
		return errors.WithStack(err)
	}
	if _, err := decoder.Token(); !errors.Is(err, io.EOF) {
		return errors.New("trailing RBAC policy data")
	}
	return nil
}

// readRBACPolicyJSONValue walks one JSON value, requiring unique keys in each
// object. The enclosing byte limit bounds width; syntax depth includes arrays.
func readRBACPolicyJSONValue(decoder *json.Decoder, depth int) error {
	if depth > 2*rbacMaxDepth+4 {
		return errors.New("RBAC policy JSON depth exceeded")
	}
	token, err := decoder.Token()
	if err != nil {
		return errors.Wrap(err, "read RBAC JSON token")
	}
	delim, ok := token.(json.Delim)
	if !ok {
		return nil
	}
	switch delim {
	case '{':
		seen := map[string]bool{}
		for decoder.More() {
			keyToken, err := decoder.Token()
			if err != nil {
				return errors.Wrap(err, "read RBAC field")
			}
			key, ok := keyToken.(string)
			if !ok || seen[key] {
				return errors.New("duplicate or invalid RBAC JSON field")
			}
			seen[key] = true
			if err := readRBACPolicyJSONValue(decoder, depth+1); err != nil {
				return errors.WithStack(err)
			}
		}
	case '[':
		for decoder.More() {
			if err := readRBACPolicyJSONValue(decoder, depth+1); err != nil {
				return errors.WithStack(err)
			}
		}
	default:
		return errors.New("invalid RBAC JSON delimiter")
	}
	if _, err := decoder.Token(); err != nil {
		return errors.Wrap(err, "close RBAC JSON value")
	}
	return nil
}
