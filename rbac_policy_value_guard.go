package utils

import "github.com/Laisky/errors/v2"

// rbacStorageValueGuard prevents copied permission values from silently falling
// back to field-by-field JSON and bypassing pointer-level policy version checks.
// The supported pointer codec never writes this sentinel field.
type rbacStorageValueGuard struct {
	Guard rbacRejectValue `json:"_storage_guard"`
}

// rbacRejectValue is an immutable sentinel for unsafe value-copy serialization.
type rbacRejectValue struct{}

// MarshalJSON rejects copied policy values. Use a permission pointer for a
// representable legacy policy, or RBACPolicyV2 for explicit grants instead.
func (rbacRejectValue) MarshalJSON() ([]byte, error) {
	return nil, errors.Wrap(ErrRBACPolicyMigrationRequired, "serialize a permission pointer or RBACPolicyV2")
}
