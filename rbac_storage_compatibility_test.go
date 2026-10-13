package utils

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// historicalRBACReader models the pre-#33 persisted fields; unknown grant fields are ignored by its JSON reader.
type historicalRBACReader struct {
	Key      string                  `json:"key"`
	FullKey  string                  `json:"full_key"`
	Children []*historicalRBACReader `json:"children"`
}

// permits reproduces the historical leaf/subtree interpretation independently of current grant-mode code.
func (p *historicalRBACReader) permits(query, parent string) bool {
	if p == nil || p.Key == "" {
		return false
	}
	full := p.FullKey
	if full == "" {
		full = p.Key
		if parent != "" {
			full = parent + "." + p.Key
		}
	}
	if len(p.Children) == 0 {
		return query == full || strings.HasPrefix(query, full+".")
	}
	for _, child := range p.Children {
		if child.permits(query, full) {
			return true
		}
	}
	return false
}

// TestRBACStorageCompatibilityNoGrantExpansion prevents old readers from treating persisted deny-only nodes as subtree grants.
func TestRBACStorageCompatibilityNoGrantExpansion(t *testing.T) {
	for _, sql := range []bool{false, true} {
		p := NewPermissionTree()
		require.False(t, p.HasPerm2("root.admin"))
		var encoded []byte
		var err error
		if sql {
			var value any
			value, err = p.Value()
			if err == nil {
				encoded = []byte(value.(string))
			}
		} else {
			encoded, err = json.Marshal(p)
		}
		if err != nil {
			continue
		} // Failing closed is required when legacy storage cannot encode the policy.
		var old historicalRBACReader
		if err := json.Unmarshal(encoded, &old); err != nil {
			continue
		}
		require.False(t, old.permits("root.admin", ""), "old reader expanded a denied grant after a mixed-version write")
	}
}

// TestRBACStorageVersionedRoundTrip preserves deny/grant semantics and rejects mixed-version readers in both directions.
func TestRBACStorageVersionedRoundTrip(t *testing.T) {
	for _, root := range []*RBACPermissionElem{NewPermissionTree(), rbacExplicitTree("root", "root.sys.read"), rbacExplicitTree("root.sys.read")} {
		policy := RBACPolicyV1{Root: root}
		value, err := policy.Value()
		require.NoError(t, err)
		var old historicalRBACReader
		require.Error(t, json.Unmarshal([]byte(value.(string)), &old), "old struct readers must reject the versioned shape")
		for _, input := range []any{value, []byte(value.(string))} {
			var restored RBACPolicyV1
			require.NoError(t, restored.Scan(input))
			for _, query := range rbacTestProbes {
				require.Equal(t, root.HasPerm2(RBACPermFullKey(query)), restored.Root.HasPerm2(RBACPermFullKey(query)))
			}
		}
	}
	p := rbacRegressionTree("read")
	legacy, err := p.Value()
	require.NoError(t, err)
	require.NotContains(t, legacy, `"grant"`)
	var old historicalRBACReader
	require.NoError(t, json.Unmarshal([]byte(legacy.(string)), &old))
	require.True(t, old.permits("root.sys.read", ""))
	require.False(t, old.permits("root.sys.admin", ""))
	var current RBACPermissionElem
	require.NoError(t, current.Scan(legacy))
	require.True(t, current.HasPerm2("root.sys.read"))
	var versioned RBACPolicyV1
	require.Error(t, versioned.Scan(legacy), "new reader must require explicit migration, not infer a version")
}

// TestRBACStorageRejectsUnknownVersions verifies failed imports leave policy state unchanged and never introduce permissive defaults.
func TestRBACStorageRejectsUnknownVersions(t *testing.T) {
	policy := RBACPolicyV1{Root: rbacExplicitTree("root.safe.read")}
	before, err := policy.Value()
	require.NoError(t, err)
	for _, raw := range []string{
		`["go-utils/rbac-policy",2,{"key":"root","grant":"subtree"}]`,
		`["go-utils/rbac-policy",1,{"key":"root"}]`,
		`["go-utils/rbac-policy",1,{"key":"root","grant":"none","unexpected":true}]`,
		`["go-utils/rbac-policy",1,{"key":"root","grant":"none","children":[{"key":"view","full_key":"root.admin","grant":"subtree"}]}]`,
		`["go-utils/rbac-policy",1,{"key":"root","grant":"none","children":[null]}]`,
		`{"key":"root","grant":"subtree"}`, `null`,
		`["go-utils/rbac-policy",1,{"key":"root","grant":"none","grant":"subtree"}]`,
		`["go-utils/rbac-policy",1,{"key":"root","grant":"none","children":[{"key":"x","grant":"none"},{"key":"x","grant":"subtree"}]}]`,
	} {
		require.Error(t, policy.Scan(raw))
		after, err := policy.Value()
		require.NoError(t, err)
		require.Equal(t, before, after)
		require.False(t, policy.Root.HasPerm2("root.admin"))
	}
	denied := NewPermissionTree()
	_, err = denied.Value()
	require.ErrorIs(t, err, ErrRBACPolicyVersionRequired)
	_, err = json.Marshal(denied)
	require.ErrorIs(t, err, ErrRBACPolicyVersionRequired)
	require.False(t, denied.HasPerm2("root.admin"))
}

// TestRBACStorageCopiedValueRejects prevents non-addressable struct values from bypassing the safe pointer serializer.
func TestRBACStorageCopiedValueRejects(t *testing.T) {
	p := NewPermissionTree()
	detached := reflect.ValueOf(p.Clone()).Elem().Interface()
	encoded, err := json.Marshal(detached)
	if err != nil {
		return
	}
	var old historicalRBACReader
	if err := json.Unmarshal(encoded, &old); err != nil {
		return
	}
	require.False(t, old.permits("root.admin", ""), "a copied value bypassed the compatibility writer guard")
}

// TestRBACStorageRejectsInvalidWriterTree prevents versioned writers from emitting a policy their own reader rejects.
func TestRBACStorageRejectsInvalidWriterTree(t *testing.T) {
	root := NewPermissionTree()
	root.Children = []*RBACPermissionElem{{Key: "x", Grant: RBACGrantNone}, {Key: "x", Grant: RBACGrantSubtree}}
	_, err := (RBACPolicyV1{Root: root}).Value()
	require.Error(t, err)
	root.Children = []*RBACPermissionElem{root}
	_, err = (RBACPolicyV1{Root: root}).Value()
	require.Error(t, err)
}
