package utils

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestRBACIdentityImportRejectsForgedDescendant checks that all persistence
// entry points reject a child outside its structural path without replacing state.
func TestRBACIdentityImportRejectsForgedDescendant(t *testing.T) {
	forged := []byte(`{"key":"root","grant":"none","children":[{"key":"reader","full_key":"root.admin","grant":"subtree"}]}`)
	decoders := map[string]func(*RBACPermissionElem) error{
		"json":        func(p *RBACPermissionElem) error { return json.Unmarshal(forged, p) },
		"scan_bytes":  func(p *RBACPermissionElem) error { return p.Scan(forged) },
		"scan_string": func(p *RBACPermissionElem) error { return p.Scan(string(forged)) },
	}
	for name, decode := range decoders {
		t.Run(name, func(t *testing.T) {
			p := rbacExplicitTree("root.safe.read")
			before, err := p.Value()
			require.NoError(t, err)
			err = decode(p)
			require.Error(t, err, "forged policy grants root.admin: %v", p.HasPerm2("root.admin.delete"))
			after, err := p.Value()
			require.NoError(t, err)
			require.Equal(t, before, after)
			require.False(t, p.HasPerm2("root.admin.delete"))
			require.True(t, p.HasPerm2("root.safe.read"))
		})
	}
}

// TestRBACIdentityDirectState checks that validation and authorization reject
// forged descendant identities even when a caller constructs the tree directly.
func TestRBACIdentityDirectState(t *testing.T) {
	p := NewPermissionTree()
	p.Children = []*RBACPermissionElem{{Key: "reader", FullKey: "root.admin", Grant: RBACGrantSubtree}}
	require.Error(t, p.Valid())
	require.False(t, p.HasPerm2("root.admin.delete"))
	require.False(t, p.HasPerm2("root.reader.delete"))
}

// TestRBACIdentityAnchoredRoundTrip retains the public standalone-subtree
// contract while ensuring descendants cannot change that subtree's identity.
func TestRBACIdentityAnchoredRoundTrip(t *testing.T) {
	p := &RBACPermissionElem{Key: "sys", Grant: RBACGrantNone, Children: []*RBACPermissionElem{{Key: "read", Grant: RBACGrantSubtree}}}
	require.NoError(t, p.FillDefault("root"))
	value, err := p.Value()
	require.NoError(t, err)
	var q RBACPermissionElem
	require.NoError(t, q.Scan(value))
	require.NoError(t, q.Valid())
	require.True(t, q.HasPerm2("root.sys.read"))
	require.False(t, q.HasPerm2("root.admin.read"))
	require.False(t, q.HasPerm2("sys.read"))
}
