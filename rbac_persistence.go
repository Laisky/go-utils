package utils

import (
	"database/sql/driver"

	"github.com/Laisky/errors/v2"

	"github.com/Laisky/go-utils/v6/json"
)

// Value implement GORM interface
func (p *RBACPermissionElem) Value() (driver.Value, error) {
	if p == nil {
		return nil, errors.Errorf("marshal RBACPermissionElem: nil receiver")
	}

	b, err := json.Marshal(p.valueSnapshot())
	if err != nil {
		return nil, errors.Wrap(err, "marshal RBACPermissionElem")
	}
	return string(b), nil
}

// Scan implement GORM interface
func (p *RBACPermissionElem) Scan(input any) error {
	if input == nil {
		return errors.Errorf("scan RBACPermissionElem: input is nil")
	}

	switch v := input.(type) {
	case []byte:
		return json.Unmarshal(v, p)
	case string:
		return json.Unmarshal([]byte(v), p)
	default:
		return errors.Errorf("scan RBACPermissionElem: unsupported type %T", input)
	}
}

// UnmarshalJSON replaces the complete authorization state, including fields
// omitted by legacy JSON. Decoding into a reused receiver must never retain an
// earlier broad grant or children. The live state is replaced only on success.
func (p *RBACPermissionElem) UnmarshalJSON(data []byte) error {
	if p == nil {
		return errors.Errorf("unmarshal RBACPermissionElem: nil receiver")
	}
	type permissionJSON RBACPermissionElem
	var decoded permissionJSON
	if err := json.Unmarshal(data, &decoded); err != nil {
		return errors.Wrap(err, "unmarshal RBACPermissionElem")
	}
	grant := decoded.Grant
	if grant == "" {
		grant = RBACGrantNone
		if len(decoded.Children) == 0 {
			grant = RBACGrantSubtree
		}
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.Title = decoded.Title
	p.Key = decoded.Key
	p.FullKey = decoded.FullKey
	p.Children = decoded.Children
	p.Grant = grant
	return nil
}
