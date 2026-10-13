package utils

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestRBACPolicyCopiedValueGuard rejects detached struct values that bypass the
// pointer serializer; old readers must never receive a deny policy as a grant.
func TestRBACPolicyCopiedValueGuard(t *testing.T) {
	root := NewPermissionTree()
	detached := reflect.ValueOf(root.Clone()).Elem().Interface()
	data, err := json.Marshal(detached)
	require.ErrorIs(t, err, ErrRBACPolicyMigrationRequired)
	require.Nil(t, data)
	require.False(t, root.HasPerm2("root.admin"))
}

// TestRBACPolicyDuplicateWriter rejects ambiguous siblings before returning bytes
// rather than emitting a versioned policy its strict reader cannot accept.
func TestRBACPolicyDuplicateWriter(t *testing.T) {
	root := NewPermissionTree()
	root.Children = []*RBACPermissionElem{
		{Key: "read", Grant: RBACGrantNone},
		{Key: "read", Grant: RBACGrantSubtree},
	}
	data, err := (RBACPolicyV2{Root: root}).MarshalJSON()
	require.Error(t, err)
	require.Nil(t, data)
}
