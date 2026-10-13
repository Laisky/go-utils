package utils

import (
	"bytes"
	"database/sql/driver"
	"encoding/json"

	"github.com/Laisky/errors/v2"
)

const (
	rbacPolicyMarker   = "go-utils/rbac"
	rbacPolicyVersion  = 2
	rbacPolicyMaxBytes = 8 << 20
	rbacPolicyMaxNodes = 65536
)

// ErrRBACPolicyMigrationRequired reports a policy that cannot be written using
// the historical leaf-grant format without changing its authorization meaning.
// Store it with RBACPolicyV2 after coordinating all policy readers and writers.
var ErrRBACPolicyMigrationRequired = errors.New("RBAC policy requires versioned storage")

// RBACPolicyV2 stores explicit grant semantics in a versioned JSON tuple:
// ["go-utils/rbac",2,root]. Historical object readers reject this shape rather
// than ignoring its version. Root is the live permission tree to snapshot when
// writing, or a newly detached tree after a successful read. Do not concurrently
// replace Root or access child fields without external synchronization.
// Use a separate versioned storage location and coordinate migration; this type
// never automatically interprets legacy objects or unknown versions.
type RBACPolicyV2 struct {
	Root *RBACPermissionElem
}

// MarshalJSON returns a bounded versioned policy with an explicit grant on every
// node. Invalid trees return an error; writing never changes the live policy.
func (policy RBACPolicyV2) MarshalJSON() ([]byte, error) {
	wire, err := snapshotRBACWire(policy.Root, false)
	if err != nil {
		return nil, errors.Wrap(err, "encode versioned RBAC policy")
	}
	data, err := json.Marshal([3]any{rbacPolicyMarker, rbacPolicyVersion, wire})
	if err != nil {
		return nil, errors.Wrap(err, "marshal versioned RBAC policy")
	}
	if len(data) > rbacPolicyMaxBytes {
		return nil, errors.New("versioned RBAC policy exceeds byte budget")
	}
	return data, nil
}

// UnmarshalJSON accepts only version 2 policies with explicit grants and
// canonical identities. Unknown versions, legacy objects, duplicate or unknown
// fields, and invalid or excessive trees leave the previous Root unchanged.
func (policy *RBACPolicyV2) UnmarshalJSON(data []byte) error {
	if policy == nil {
		return errors.New("decode versioned RBAC policy: nil receiver")
	}
	if len(data) > rbacPolicyMaxBytes {
		return errors.New("versioned RBAC policy exceeds byte budget")
	}
	var fields []json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return errors.Wrap(err, "decode RBAC policy tuple")
	}
	if len(fields) != 3 {
		return errors.New("invalid RBAC policy tuple length")
	}
	var marker string
	var version int
	if err := json.Unmarshal(fields[0], &marker); err != nil {
		return errors.Wrap(err, "decode RBAC policy marker")
	}
	if err := json.Unmarshal(fields[1], &version); err != nil {
		return errors.Wrap(err, "decode RBAC policy version")
	}
	if marker != rbacPolicyMarker || version != rbacPolicyVersion {
		return errors.New("unsupported RBAC policy marker or version")
	}
	if err := validateRBACPolicyJSON(fields[2]); err != nil {
		return errors.WithStack(err)
	}
	var wire *permissionJSON
	decoder := json.NewDecoder(bytes.NewReader(fields[2]))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&wire); err != nil {
		return errors.Wrap(err, "decode RBAC policy root")
	}
	nodes := 0
	if err := requireRBACWireGrants(wire, 0, &nodes); err != nil {
		return errors.WithStack(err)
	}
	decoded, err := decodePermissionTree(wire, 0)
	if err != nil {
		return errors.Wrap(err, "decode explicit RBAC tree")
	}
	if err := decoded.Valid(); err != nil {
		return errors.Wrap(err, "validate versioned RBAC tree")
	}
	policy.Root = decoded
	return nil
}

// Value returns the versioned JSON tuple for SQL storage, or an error if the
// policy cannot be safely represented. It does not migrate a legacy column.
func (policy RBACPolicyV2) Value() (driver.Value, error) {
	data, err := policy.MarshalJSON()
	if err != nil {
		return nil, errors.Wrap(err, "store versioned RBAC policy")
	}
	return string(data), nil
}

// Scan decodes versioned SQL string or byte data without changing the receiver
// on error. SQL NULL, legacy JSON objects, and unknown versions are rejected.
func (policy *RBACPolicyV2) Scan(value any) error {
	switch data := value.(type) {
	case string:
		if len(data) > rbacPolicyMaxBytes {
			return errors.New("versioned RBAC policy exceeds byte budget")
		}
		return errors.Wrap(policy.UnmarshalJSON([]byte(data)), "scan versioned RBAC policy")
	case []byte:
		return errors.Wrap(policy.UnmarshalJSON(data), "scan versioned RBAC policy")
	default:
		return errors.Errorf("scan versioned RBAC policy: unsupported type %T", value)
	}
}

// MarshalJSON retains the historical object format only when old leaf-based
// readers assign the same grants. Otherwise it returns an explicit migration
// error, never bytes that silently expand permissions in an older instance.
//
// Deprecated: Use RBACPolicyV2 for explicit grant persistence. Coordinate storage
// versioning and rollback before enabling new writers; do not restore unsafe
// authorization defaults to work around ErrRBACPolicyMigrationRequired.
func (p *RBACPermissionElem) MarshalJSON() ([]byte, error) {
	wire, err := snapshotRBACWire(p, true)
	if err != nil {
		return nil, errors.Wrap(err, "encode legacy RBAC policy")
	}
	data, err := json.Marshal(wire)
	if err != nil {
		return nil, errors.Wrap(err, "marshal legacy RBAC policy")
	}
	return data, nil
}

// snapshotRBACWire copies one tree under its root read lock. Legacy encoding
// must exactly match leaf-based grants; versioned encoding materializes grants
// and applies additional resource limits. Neither mode trusts forged identities.
func snapshotRBACWire(p *RBACPermissionElem, legacy bool) (*permissionJSON, error) {
	if p == nil {
		return nil, errors.New("encode RBAC policy: nil root")
	}
	p.mu.RLock()
	defer p.mu.RUnlock()
	nodes, textBytes := 0, 0
	return copyRBACWire(p, "", 0, legacy, &nodes, &textBytes)
}

// copyRBACWire validates and copies a locked tree using its original grant
// meaning. Budgets bound new-format allocations, and depth also rejects cycles.
func copyRBACWire(p *RBACPermissionElem, parent RBACPermFullKey, depth int,
	legacy bool, nodes, textBytes *int) (*permissionJSON, error) {
	if p == nil || depth > rbacMaxDepth || !p.validGrantMode() {
		return nil, errors.New("invalid RBAC policy node or depth")
	}
	current, valid := p.canonicalRBACIdentity(parent, depth)
	if !valid {
		return nil, errors.New("invalid RBAC policy identity")
	}
	*nodes++
	if !legacy {
		// Count before allocating child slices; overflow is avoided by checking
		// each field against the remaining byte allowance individually.
		if *nodes > rbacPolicyMaxNodes || len(p.Children) > rbacPolicyMaxNodes-*nodes {
			return nil, errors.New("versioned RBAC policy exceeds node budget")
		}
		for _, size := range []int{len(p.Title), len(p.Key), len(p.FullKey)} {
			if size > rbacPolicyMaxBytes-*textBytes {
				return nil, errors.New("versioned RBAC policy exceeds text budget")
			}
			*textBytes += size
		}
	}
	mode := p.explicitGrantMode()
	wire := &permissionJSON{Title: p.Title, Key: p.Key, FullKey: p.FullKey, Grant: mode}
	if legacy {
		expected := RBACGrantNone
		if len(p.Children) == 0 {
			expected = RBACGrantSubtree
		}
		if mode != expected {
			return nil, errors.WithStack(ErrRBACPolicyMigrationRequired)
		}
		// Preserve the historical representation, rather than relying on an
		// old reader to ignore a new field carrying a different security model.
		wire.Grant = ""
	}
	if p.Children != nil {
		wire.Children = make([]*permissionJSON, len(p.Children))
		for i, child := range p.Children {
			converted, err := copyRBACWire(child, current, depth+1, legacy, nodes, textBytes)
			if err != nil {
				return nil, errors.Wrapf(err, "encode RBAC child %d", i)
			}
			wire.Children[i] = converted
		}
	}
	return wire, nil
}

// requireRBACWireGrants rejects missing or unknown grants rather than adapting
// them using legacy implicit defaults. It also bounds the decoded tree size.
func requireRBACWireGrants(wire *permissionJSON, depth int, nodes *int) error {
	if wire == nil || depth > rbacMaxDepth {
		return errors.New("invalid versioned RBAC policy depth or null node")
	}
	*nodes++
	if *nodes > rbacPolicyMaxNodes {
		return errors.New("versioned RBAC policy exceeds node budget")
	}
	if wire.Grant != RBACGrantNone && wire.Grant != RBACGrantSubtree {
		return errors.New("versioned RBAC policy requires an explicit grant on every node")
	}
	for _, child := range wire.Children {
		if err := requireRBACWireGrants(child, depth+1, nodes); err != nil {
			return errors.WithStack(err)
		}
	}
	return nil
}
