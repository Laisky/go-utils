package utils

import (
	"bytes"
	"database/sql/driver"
	"encoding/json"
	"io"

	"github.com/Laisky/errors/v2"
)

const (
	rbacPolicyFormat   = "go-utils/rbac-policy"
	rbacPolicyVersion  = 1
	rbacPolicyMaxBytes = 8 << 20
	rbacPolicyMaxNodes = 10000
)

// ErrRBACPolicyVersionRequired reports that an explicit grant cannot be represented
// in historical storage without changing what an old reader authorizes.
var ErrRBACPolicyVersionRequired = errors.New("RBAC policy requires versioned storage")

// RBACPolicyV1 stores an explicit-grant tree in a versioned, non-legacy JSON tuple.
// Keep it in a separate versioned column/keyspace and upgrade readers before writers.
// Old object readers reject its array shape, rather than ignoring a version field.
// Root remains governed by RBACPermissionElem's locking rules; do not concurrently
// replace Root or invoke Scan/UnmarshalJSON on the same wrapper without synchronization.
// No automatic legacy conversion or permissive grant defaults are performed by its reader.
type RBACPolicyV1 struct {
	Root *RBACPermissionElem
}

// rbacStorageValueGuard prevents a non-addressable copy of a mutex-bearing policy
// from falling back to unsafe field-by-field JSON. Pointers use the explicit safe
// serializer below; copied values cannot lock the original tree and are rejected.
type rbacStorageValueGuard struct {
	// Guard has no mutable state and is excluded by the supported pointer codec.
	Guard rbacRejectValue `json:"_storage_guard"`
}

// rbacRejectValue is the always-present fallback field of copied policy values.
type rbacRejectValue struct{}

// MarshalJSON rejects copied permission values; serialize a pointer or RBACPolicyV1
// rather than bypassing grant-version checks through a struct copy.
func (rbacRejectValue) MarshalJSON() ([]byte, error) {
	return nil, errors.Wrap(ErrRBACPolicyVersionRequired, "serialize a permission pointer or RBACPolicyV1")
}

// MarshalJSON returns only the subset of historical object storage whose inferred
// leaf grants exactly match the current tree. Nonrepresentable explicit modes fail
// closed instead of emitting fields that old readers ignore. Missing legacy key
// fields remain serializable but still cannot authorize requests.
//
// Deprecated: Use RBACPolicyV1 in separately versioned policy storage for explicit
// grants. Do not dual-write nonrepresentable policy states into legacy columns.
func (p *RBACPermissionElem) MarshalJSON() ([]byte, error) {
	wire, err := snapshotRBACWire(p, true)
	if err != nil {
		return nil, errors.Wrap(err, "encode legacy RBAC policy")
	}
	out, err := json.Marshal(wire)
	if err != nil {
		return nil, errors.Wrap(err, "marshal legacy RBAC policy")
	}
	return out, nil
}

// snapshotRBACWire copies authorization fields while holding the root lock, rejecting
// invalid depth or identities before serialization. legacy requires exact pre-#33
// grant semantics; versioned output materializes every grant and has a node budget.
func snapshotRBACWire(p *RBACPermissionElem, legacy bool) (*permissionJSON, error) {
	if p == nil {
		return nil, errors.New("nil RBAC policy root")
	}
	p.mu.RLock()
	defer p.mu.RUnlock()
	count := 0
	return encodeRBACWire(p, "", 0, &count, legacy)
}

// encodeRBACWire builds a detached wire tree under the caller-held root lock.
// It validates identities and grant modes without silently pruning or normalizing
// invalid input; count bounds new-format policy work and depth also catches cycles.
func encodeRBACWire(p *RBACPermissionElem, parent RBACPermFullKey, depth int, count *int, legacy bool) (*permissionJSON, error) {
	if p == nil || depth > rbacMaxDepth || !p.validGrantMode() {
		return nil, errors.New("invalid RBAC policy structure")
	}
	*count++
	if !legacy && *count > rbacPolicyMaxNodes {
		return nil, errors.New("RBAC policy node budget exceeded")
	}
	full, valid := p.canonicalRBACIdentity(parent, depth)
	if !valid {
		if !legacy || p.Key != "" || p.FullKey != "" {
			return nil, errors.New("invalid RBAC policy identity")
		}
		full = parent
	}
	mode := p.explicitGrantMode()
	if legacy {
		inferred := RBACGrantNone
		if len(p.Children) == 0 {
			inferred = RBACGrantSubtree
		}
		if mode != inferred {
			return nil, errors.WithStack(ErrRBACPolicyVersionRequired)
		}
		mode = ""
	}
	wire := &permissionJSON{Title: p.Title, Key: p.Key, FullKey: p.FullKey, Grant: mode}
	if !legacy {
		wire.FullKey = full
	}
	if p.Children != nil {
		if !legacy && len(p.Children) > rbacPolicyMaxNodes-*count {
			return nil, errors.New("RBAC policy node budget exceeded")
		}
		wire.Children = make([]*permissionJSON, len(p.Children))
		for i, child := range p.Children {
			var err error
			wire.Children[i], err = encodeRBACWire(child, full, depth+1, count, legacy)
			if err != nil {
				return nil, errors.Wrapf(err, "encode RBAC child %d", i)
			}
		}
	}
	return wire, nil
}

// MarshalJSON writes the explicit version-1 tuple without calling the deprecated
// legacy serializer. Errors never return a partial policy. The format is an
// opt-in storage contract, not a drop-in representation for an existing column.
func (p RBACPolicyV1) MarshalJSON() ([]byte, error) {
	wire, err := snapshotRBACWire(p.Root, false)
	if err != nil {
		return nil, errors.Wrap(err, "snapshot versioned RBAC policy")
	}
	count := 0
	if err := validateExplicitRBACWire(wire, 0, &count); err != nil {
		return nil, errors.WithStack(err)
	}
	out, err := json.Marshal([3]any{rbacPolicyFormat, rbacPolicyVersion, wire})
	if err != nil {
		return nil, errors.Wrap(err, "marshal versioned RBAC policy")
	}
	if len(out) > rbacPolicyMaxBytes {
		return nil, errors.New("RBAC policy byte budget exceeded")
	}
	return out, nil
}

// UnmarshalJSON accepts exactly the known versioned tuple and explicit grants.
// Unknown versions, legacy objects, malformed identities or structure leave the
// receiver unchanged. Decode errors must abort authorization, not reuse old state.
func (p *RBACPolicyV1) UnmarshalJSON(data []byte) error {
	if p == nil {
		return errors.New("nil RBACPolicyV1 receiver")
	}
	if len(data) > rbacPolicyMaxBytes {
		return errors.New("RBAC policy byte budget exceeded")
	}
	if err := validateRBACPolicyJSON(data); err != nil {
		return errors.WithStack(err)
	}
	var tuple []json.RawMessage
	if err := json.Unmarshal(data, &tuple); err != nil {
		return errors.Wrap(err, "decode RBAC policy tuple")
	}
	if len(tuple) != 3 {
		return errors.New("invalid RBAC policy tuple length")
	}
	var format string
	var version int
	if err := json.Unmarshal(tuple[0], &format); err != nil {
		return errors.Wrap(err, "decode RBAC format")
	}
	if err := json.Unmarshal(tuple[1], &version); err != nil {
		return errors.Wrap(err, "decode RBAC version")
	}
	if format != rbacPolicyFormat || version != rbacPolicyVersion {
		return errors.New("unsupported RBAC policy format or version")
	}
	var wire *permissionJSON
	decoder := json.NewDecoder(bytes.NewReader(tuple[2]))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&wire); err != nil {
		return errors.Wrap(err, "decode explicit RBAC policy")
	}
	if err := decoder.Decode(new(any)); !errors.Is(err, io.EOF) {
		return errors.New("trailing RBAC policy data")
	}
	count := 0
	if err := validateExplicitRBACWire(wire, 0, &count); err != nil {
		return errors.WithStack(err)
	}
	root, err := decodePermissionTree(wire, 0)
	if err != nil {
		return errors.Wrap(err, "decode RBAC tree")
	}
	if err := root.validateRBACIdentities("", 0, false); err != nil {
		return errors.Wrap(err, "validate RBAC policy identity")
	}
	p.Root = root
	return nil
}

// validateExplicitRBACWire requires all grant modes to be explicit and bounds the
// number/depth of nodes. Duplicate sibling identities are rejected as ambiguous.
func validateExplicitRBACWire(wire *permissionJSON, depth int, count *int) error {
	if wire == nil || depth > rbacMaxDepth {
		return errors.New("invalid explicit RBAC tree")
	}
	*count++
	if *count > rbacPolicyMaxNodes {
		return errors.New("RBAC policy node budget exceeded")
	}
	if wire.Grant != RBACGrantNone && wire.Grant != RBACGrantSubtree {
		return errors.New("versioned RBAC grant must be explicit")
	}
	seen := make(map[RBACPermKey]bool, len(wire.Children))
	for _, child := range wire.Children {
		if child == nil || seen[child.Key] {
			return errors.New("null or duplicate RBAC child")
		}
		seen[child.Key] = true
		if err := validateExplicitRBACWire(child, depth+1, count); err != nil {
			return errors.WithStack(err)
		}
	}
	return nil
}

// Value serializes the versioned policy for a separate SQL policy column.
func (p RBACPolicyV1) Value() (driver.Value, error) {
	data, err := p.MarshalJSON()
	if err != nil {
		return nil, errors.Wrap(err, "value RBACPolicyV1")
	}
	return string(data), nil
}

// Scan decodes a string or byte slice containing a versioned policy. Legacy object
// storage must be migrated explicitly from its authoritative grant set instead.
func (p *RBACPolicyV1) Scan(value any) error {
	var data []byte
	switch v := value.(type) {
	case string:
		data = []byte(v)
	case []byte:
		data = v
	default:
		return errors.Errorf("unsupported RBACPolicyV1 SQL value %T", value)
	}
	if err := p.UnmarshalJSON(data); err != nil {
		return errors.Wrap(err, "scan RBACPolicyV1")
	}
	return nil
}
