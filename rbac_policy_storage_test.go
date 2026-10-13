package utils

import (
	"encoding/json"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestRBACPolicyV2RoundTrip preserves explicit grants and denied structural
// leaves across both JSON and SQL, and proves that old object readers reject it.
func TestRBACPolicyV2RoundTrip(t *testing.T) {
	for mask := uint64(0); mask < 64; mask++ {
		grants := rbacMaskGrants(mask)
		original := rbacExplicitTree(grants...)
		original.Children = append(original.Children, &RBACPermissionElem{
			Key: "denied", FullKey: "root.denied", Title: "retained metadata", Grant: RBACGrantNone,
		})
		policy := RBACPolicyV2{Root: original}
		encoded, err := json.Marshal(policy)
		require.NoError(t, err)
		require.True(t, strings.HasPrefix(string(encoded), `["go-utils/rbac",2,`))
		var old pre33RBACReader
		require.Error(t, json.Unmarshal(encoded, &old), "old readers must reject the new shape")
		oldAPI := rbacExplicitTree("root.safe")
		require.Error(t, oldAPI.Scan(encoded), "legacy Scan must not autodetect new policies")
		require.True(t, oldAPI.HasPerm2("root.safe"))
		require.False(t, oldAPI.HasPerm2("root.admin"))
		var decoded RBACPolicyV2
		require.NoError(t, json.Unmarshal(encoded, &decoded))
		value, err := policy.Value()
		require.NoError(t, err)
		for _, input := range []any{value, []byte(value.(string))} {
			var stored RBACPolicyV2
			require.NoError(t, stored.Scan(input))
			for _, tree := range []*RBACPermissionElem{decoded.Root, stored.Root} {
				rbacAssertPermissions(t, tree, rbacTestProbes, func(q string) bool { return rbacOracle(grants, q) })
				require.Equal(t, RBACGrantNone, tree.GetElemByKey("root.denied").Grant)
				require.Equal(t, "retained metadata", tree.GetElemByKey("root.denied").Title)
				require.NoError(t, tree.FillDefault(""))
				rbacAssertPermissions(t, tree, rbacTestProbes, func(q string) bool { return rbacOracle(grants, q) })
			}
		}
		// Decoded data owns a separate tree and cannot mutate the original.
		decoded.Root.Title = "changed copy"
		require.NotEqual(t, decoded.Root.Title, original.Title)
	}
}

// TestRBACPolicyV2RejectsAmbiguity ensures malformed or downgraded policies never
// replace the previous authorized state, even if the envelope version is valid.
func TestRBACPolicyV2RejectsAmbiguity(t *testing.T) {
	cases := []string{
		`null`, `[]`, `{}`, `{"key":"root"}`,
		`["go-utils/rbac",1,{"key":"root","grant":"subtree"}]`,
		`["other",2,{"key":"root","grant":"subtree"}]`,
		`["go-utils/rbac",2,null]`,
		`["go-utils/rbac",2,{"key":"root"}]`,
		`["go-utils/rbac",2,{"key":"root","grant":""}]`,
		`["go-utils/rbac",2,{"key":"root","grant":"allow"}]`,
		`["go-utils/rbac",2,{"key":"root","grant":"none","grant":"subtree"}]`,
		`["go-utils/rbac",2,{"key":"root","Grant":"subtree"}]`,
		`["go-utils/rbac",2,{"key":"root","grant":"none","future":"x"}]`,
		`["go-utils/rbac",2,{"key":"root","grant":"none","children":[null]}]`,
		`["go-utils/rbac",2,{"key":"root","grant":"none","children":[{"key":"admin"}]}]`,
		`["go-utils/rbac",2,{"key":"root","grant":"none","children":[{"key":"read","grant":"subtree","full_key":"root.admin"}]}]`,
		`["go-utils/rbac",2,{"key":"root","grant":"subtree"},true]`,
		`["go-utils/rbac",2,{"key":"root","grant":"subtree"}] true`,
		strings.Repeat(" ", rbacPolicyMaxBytes+1),
	}
	root := `{"key":"n","grant":"none"}`
	for range rbacMaxDepth + 1 {
		root = `{"key":"n","grant":"none","children":[` + root + `]}`
	}
	cases = append(cases, `["go-utils/rbac",2,`+root+`]`)
	for i, raw := range cases {
		for _, viaSQL := range []bool{false, true} {
			previous := rbacExplicitTree("root.safe")
			policy := RBACPolicyV2{Root: previous}
			var err error
			if viaSQL {
				err = policy.Scan(raw)
			} else {
				err = json.Unmarshal([]byte(raw), &policy)
			}
			require.Error(t, err, "case %d SQL=%v", i, viaSQL)
			require.Same(t, previous, policy.Root)
			require.True(t, policy.Root.HasPerm2("root.safe"))
			require.False(t, policy.Root.HasPerm2("root.admin"))
		}
	}
	var nilPolicy *RBACPolicyV2
	require.Error(t, nilPolicy.Scan(`[]`))
	require.Error(t, nilPolicy.UnmarshalJSON([]byte(`[]`)))
	var policy RBACPolicyV2
	require.Error(t, policy.Scan(nil))
	require.Error(t, policy.Scan(4))
	_, err := policy.Value()
	require.Error(t, err)
}

// TestRBACPolicyLegacyCompatibility keeps pre-#33 wire representation and grant
// meaning in both directions for policies that the historical model can express.
func TestRBACPolicyLegacyCompatibility(t *testing.T) {
	inputs := []string{
		`{"title":"root","key":"root"}`,
		`{"title":"root","key":"root","children":[{"title":"read","key":"read"}]}`,
		`{"title":"anchored","key":"sys","full_key":"root.sys","children":[{"title":"read","key":"read","full_key":"root.sys.read"}]}`,
		`{"title":"wildcard","key":"root","children":[{"key":"*"}]}`,
	}
	for _, raw := range inputs {
		var old pre33RBACReader
		require.NoError(t, json.Unmarshal([]byte(raw), &old))
		var current RBACPermissionElem
		require.NoError(t, current.Scan(raw))
		for _, fill := range []bool{false, true} {
			if fill {
				require.NoError(t, current.FillDefault(""))
			}
			out, err := current.Value()
			require.NoError(t, err)
			require.NotContains(t, out.(string), `"grant"`)
			var older pre33RBACReader
			require.NoError(t, json.Unmarshal([]byte(out.(string)), &older))
			for _, probe := range append(rbacTestProbes, "root.read", "root.sys.read.child") {
				require.Equal(t, old.allows(probe, ""), current.HasPerm2(RBACPermFullKey(probe)))
				require.Equal(t, old.allows(probe, ""), older.allows(probe, ""))
			}
		}
	}
	for _, bad := range []*RBACPermissionElem{
		NewPermissionTree(),
		{Key: "root", Grant: RBACGrantNone, Children: []*RBACPermissionElem{{Key: "empty", Grant: RBACGrantNone}}},
		{Key: "root", Grant: RBACGrantSubtree, Children: []*RBACPermissionElem{{Key: "read", Grant: RBACGrantSubtree}}},
	} {
		out, err := bad.Value()
		require.ErrorIs(t, err, ErrRBACPolicyMigrationRequired)
		require.Nil(t, out)
		bytes, err := json.Marshal(bad)
		require.ErrorIs(t, err, ErrRBACPolicyMigrationRequired)
		require.Nil(t, bytes)
	}
}

// TestRBACPolicyV2SnapshotConcurrency verifies serialization uses the tree root
// lock while grants are mutated through supported root methods.
func TestRBACPolicyV2SnapshotConcurrency(t *testing.T) {
	root := rbacExplicitTree("root.read")
	policy := RBACPolicyV2{Root: root}
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		for range 50 {
			root.UnionAndOverwriteBy(rbacExplicitTree("root.write"))
			root.Cut("root.write")
		}
	}()
	go func() {
		defer wg.Done()
		for range 50 {
			raw, err := policy.MarshalJSON()
			if !require.NoError(t, err) {
				return
			}
			var restored RBACPolicyV2
			if !require.NoError(t, restored.UnmarshalJSON(raw)) {
				return
			}
			require.True(t, restored.Root.HasPerm2("root.read"))
			require.False(t, restored.Root.HasPerm2("root.admin"))
		}
	}()
	wg.Wait()
}

// TestRBACPolicyV2WriteBudgets refuses invalid or excessive trees rather than
// truncating them into a potentially more permissive leaf representation.
func TestRBACPolicyV2WriteBudgets(t *testing.T) {
	excessive := NewPermissionTree()
	excessive.Title = strings.Repeat("x", rbacPolicyMaxBytes+1)
	for _, root := range []*RBACPermissionElem{
		excessive, {Key: "root", Grant: RBACGrantMode("bad")},
		{Key: "root", Grant: RBACGrantNone, Children: []*RBACPermissionElem{{Key: "read", FullKey: "root.admin"}}},
		{Key: "root", Children: []*RBACPermissionElem{nil}},
		{Key: "root", Children: make([]*RBACPermissionElem, rbacPolicyMaxNodes)},
	} {
		data, err := (RBACPolicyV2{Root: root}).MarshalJSON()
		require.Error(t, err)
		require.Nil(t, data)
	}
	cyclic := NewPermissionTree()
	cyclic.Children = []*RBACPermissionElem{cyclic}
	data, err := (RBACPolicyV2{Root: cyclic}).MarshalJSON()
	require.Error(t, err)
	require.Nil(t, data)
}
