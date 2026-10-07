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
		return errors.Wrap(json.Unmarshal(v, p), "scan RBACPermissionElem")
	case string:
		return errors.Wrap(json.Unmarshal([]byte(v), p), "scan RBACPermissionElem")
	default:
		return errors.Errorf("scan RBACPermissionElem: unsupported type %T", input)
	}
}

// permissionJSON is a detached wire tree. Its child type deliberately does not
// call UnmarshalJSON recursively, so validation uses one consistent depth limit.
type permissionJSON struct {
	Title    string            `json:"title"`
	Key      RBACPermKey       `json:"key,omitempty"`
	FullKey  RBACPermFullKey   `json:"full_key,omitempty"`
	Children []*permissionJSON `json:"children,omitempty"`
	Grant    RBACGrantMode     `json:"grant,omitempty"`
}

// decodePermissionTree validates structural state and adapts legacy grant modes.
// Missing keys retain legacy decode compatibility; Valid reports missing keys.
func decodePermissionTree(wire *permissionJSON, depth int) (*RBACPermissionElem, error) {
	if wire == nil {
		return nil, errors.New("unmarshal RBACPermissionElem: null permission node")
	}
	if depth > rbacMaxDepth {
		return nil, errors.Errorf("tree depth exceeds maximum %d", rbacMaxDepth)
	}
	p := &RBACPermissionElem{Title: wire.Title, Key: wire.Key, FullKey: wire.FullKey, Grant: wire.Grant}
	if !p.validGrantMode() {
		return nil, errors.New("unmarshal RBACPermissionElem: invalid grant mode")
	}
	if wire.Children != nil {
		p.Children = make([]*RBACPermissionElem, len(wire.Children))
		for i, child := range wire.Children {
			decoded, err := decodePermissionTree(child, depth+1)
			if err != nil {
				return nil, errors.Wrapf(err, "decode child %d", i)
			}
			p.Children[i] = decoded
		}
	}
	p.Grant = p.explicitGrantMode()
	return p, nil
}

// UnmarshalJSON validates a detached tree before replacing live authorization
// state. Null children, invalid grants, and excessive depth leave the receiver
// unchanged. Missing fields in valid legacy JSON still replace old state.
func (p *RBACPermissionElem) UnmarshalJSON(data []byte) error {
	if p == nil {
		return errors.New("unmarshal RBACPermissionElem: nil receiver")
	}
	var wire *permissionJSON
	if err := json.Unmarshal(data, &wire); err != nil {
		return errors.Wrap(err, "unmarshal RBACPermissionElem")
	}
	decoded, err := decodePermissionTree(wire, 0)
	if err != nil {
		return errors.WithStack(err)
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.Title = decoded.Title
	p.Key = decoded.Key
	p.FullKey = decoded.FullKey
	p.Children = decoded.Children
	p.Grant = decoded.Grant
	return nil
}
