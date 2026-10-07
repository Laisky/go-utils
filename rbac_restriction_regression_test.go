package utils

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// rbacRegressionTree builds a legacy policy with the supplied sys child keys.
// Invalid fixture construction panics rather than producing a misleading test.
func rbacRegressionTree(keys ...string) *RBACPermissionElem {
	p := &RBACPermissionElem{Key: "root", Children: []*RBACPermissionElem{{Key: "sys"}}}
	for _, k := range keys {
		p.Children[0].Children = append(p.Children[0].Children, &RBACPermissionElem{Key: RBACPermKey(k)})
	}
	if err := p.FillDefault(""); err != nil {
		panic(err)
	}
	return p
}

// TestRBACRestrictionRegression reproduces the five original escalation paths.
// Each case asserts both its precondition and the resulting effective denial.
func TestRBACRestrictionRegression(t *testing.T) {
	cases := []struct {
		name  string
		apply func(*RBACPermissionElem)
	}{
		{"disjoint_intersection", func(p *RBACPermissionElem) { p.Intersection(rbacRegressionTree("write")) }},
		{"disjoint_overwrite_intersection", func(p *RBACPermissionElem) { p.OverwriteBy(rbacRegressionTree("write"), true) }},
		{"cut_last_child", func(p *RBACPermissionElem) { p.Cut("root.sys.read") }},
		{"cut_wildcard_descendants", func(p *RBACPermissionElem) { p.Cut("root.sys.*") }},
		{"cut_last_root_child", func(p *RBACPermissionElem) { p.Cut("root.sys") }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := rbacRegressionTree("read")
			for _, key := range []RBACPermFullKey{"root", "root.sys", "root.sys.admin"} {
				require.Falsef(t, p.HasPerm2(key), "invalid precondition: already grants %s", key)
			}
			tc.apply(p)
			for _, key := range []RBACPermFullKey{"root", "root.sys", "root.sys.admin", "root.sys.read"} {
				require.Falsef(t, p.HasPerm2(key), "restriction grants %s", key)
			}
		})
	}
}
