package utils

import (
	"bytes"
	"encoding/json"
	"io"

	"github.com/Laisky/errors/v2"
)

// validateRBACPolicyJSON rejects duplicate, unknown, and incorrectly cased field
// names before decoding a versioned permission tree. Token nesting is bounded
// independently of the decoder's much larger general-purpose JSON limit.
func validateRBACPolicyJSON(data []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	objects := 0
	if err := readRBACPolicyJSONValue(decoder, 0, &objects); err != nil {
		return errors.Wrap(err, "validate RBAC policy JSON")
	}
	if _, err := decoder.Token(); !errors.Is(err, io.EOF) {
		if err != nil {
			return errors.Wrap(err, "read RBAC policy end")
		}
		return errors.New("unexpected trailing RBAC policy JSON")
	}
	return nil
}

// readRBACPolicyJSONValue consumes a complete JSON value and validates object
// keys and nesting. Schema and grant validation is completed by the typed reader.
func readRBACPolicyJSONValue(decoder *json.Decoder, depth int, objects *int) error {
	if depth > 2*rbacMaxDepth+2 {
		return errors.New("RBAC policy JSON exceeds nesting budget")
	}
	token, err := decoder.Token()
	if err != nil {
		return errors.Wrap(err, "read RBAC policy token")
	}
	delim, container := token.(json.Delim)
	if !container {
		return nil
	}
	switch delim {
	case '{':
		*objects++
		if *objects > rbacPolicyMaxNodes {
			return errors.New("RBAC policy JSON exceeds node budget")
		}
		seen := make(map[string]struct{}, 5)
		for decoder.More() {
			token, err := decoder.Token()
			if err != nil {
				return errors.Wrap(err, "read RBAC policy field")
			}
			key, ok := token.(string)
			if !ok {
				return errors.New("invalid RBAC policy field name")
			}
			switch key {
			case "title", "key", "full_key", "children", "grant":
			default:
				return errors.New("unknown RBAC policy field")
			}
			if _, duplicate := seen[key]; duplicate {
				return errors.New("duplicate RBAC policy field")
			}
			seen[key] = struct{}{}
			if err := readRBACPolicyJSONValue(decoder, depth+1, objects); err != nil {
				return errors.WithStack(err)
			}
		}
	case '[':
		for decoder.More() {
			if err := readRBACPolicyJSONValue(decoder, depth+1, objects); err != nil {
				return errors.WithStack(err)
			}
		}
	default:
		return errors.New("unexpected RBAC policy delimiter")
	}
	if _, err := decoder.Token(); err != nil {
		return errors.Wrap(err, "read RBAC policy closing delimiter")
	}
	return nil
}
