package utils

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRBACPermissionElemFullKey_Parent(t *testing.T) {
	tests := []struct {
		name string
		p    RBACPermFullKey
		want RBACPermFullKey
	}{
		{"0", RBACPermFullKey(""), ""},
		{"1", RBACPermFullKey("a"), ""},
		{"2", RBACPermFullKey("a.b"), "a"},
		{"3", RBACPermFullKey("a.b.c"), "a.b"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.p.Parent(); got != tt.want {
				t.Errorf("RBACPermissionElemFullKey.Parent() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestRBACPermissionElemFullKey_Append(t *testing.T) {
	type args struct {
		key RBACPermKey
	}
	tests := []struct {
		name string
		p    RBACPermFullKey
		args args
		want RBACPermFullKey
	}{
		{"0", RBACPermFullKey("a.b"), args{RBACPermKey("c")}, RBACPermFullKey("a.b.c")},
		{"0", RBACPermFullKey(""), args{RBACPermKey("c")}, RBACPermFullKey("c")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.p.Append(tt.args.key); got != tt.want {
				t.Errorf("RBACPermissionElemFullKey.Append() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestRBACPermissionGrantsRequired(t *testing.T) {
	tests := []struct {
		name       string
		permission RBACPermFullKey
		required   RBACPermFullKey
		want       bool
	}{
		{"exact_match", RBACPermFullKey("a.b"), RBACPermFullKey("a.b"), true},
		{"empty_required", RBACPermFullKey("a.b"), RBACPermFullKey(""), true},
		{"ancestor_grants_descendant", RBACPermFullKey("a"), RBACPermFullKey("a.b"), true},
		{"descendant_not_grant_ancestor", RBACPermFullKey("a.b"), RBACPermFullKey("a"), false},
		{"segment_boundary_mismatch", RBACPermFullKey("root.sys"), RBACPermFullKey("root.sysadmin"), false},
		{"wildcard_grants_descendant", RBACPermFullKey("root.sys.*"), RBACPermFullKey("root.sys.read"), true},
		{"wildcard_not_grant_parent", RBACPermFullKey("root.sys.*"), RBACPermFullKey("root.sys"), false},
		{"wildcard_not_grant_sibling_segment", RBACPermFullKey("root.sys.*"), RBACPermFullKey("root.sysadmin"), false},
		{"empty_permission_not_grant_nonempty", RBACPermFullKey(""), RBACPermFullKey("root"), false},
		{"both_empty", RBACPermFullKey(""), RBACPermFullKey(""), true},
		{"wildcard_grants_deep_descendant", RBACPermFullKey("root.*"), RBACPermFullKey("root.a.b.c"), true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, rbacPermissionGrantsRequired(tt.permission, tt.required))
		})
	}
}

func TestRBACCutTargetMatchesNode(t *testing.T) {
	tests := []struct {
		name   string
		target RBACPermFullKey
		node   RBACPermFullKey
		want   bool
	}{
		{"exact_match", "root.a", "root.a", true},
		{"no_match", "root.a", "root.b", false},
		{"empty_target", "", "root.a", false},
		{"empty_node", "root.a", "", false},
		{"both_empty", "", "", false},
		{"wildcard_matches_child", "root.a.*", "root.a.b", true},
		{"wildcard_not_match_parent", "root.a.*", "root.a", false},
		{"wildcard_not_match_segment_prefix", "root.sys.*", "root.sysadmin", false},
		{"exact_not_match_child", "root.a", "root.a.b", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, rbacCutTargetMatchesNode(tt.target, tt.node))
		})
	}
}

func TestHasRBACHierarchicalPrefix(t *testing.T) {
	tests := []struct {
		name      string
		childKey  string
		parentKey string
		want      bool
	}{
		{"child_of_parent", "root.sys.read", "root.sys", true},
		{"not_child", "root.sysadmin", "root.sys", false},
		{"empty_parent", "root.sys", "", true},
		{"empty_child", "", "root.sys", false},
		{"same_key", "root.sys", "root.sys", false},
		{"child_shorter", "root", "root.sys", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, hasRBACHierarchicalPrefix(tt.childKey, tt.parentKey))
		})
	}
}

func TestRBACPermissionElem_CutAvoidSegmentMismatch(t *testing.T) {
	p := &RBACPermissionElem{
		Key: "root",
		Children: []*RBACPermissionElem{
			{
				Key: "sys",
				Children: []*RBACPermissionElem{
					{Key: "read"},
					{Key: "write"},
				},
			},
			{
				Key: "sysadmin",
				Children: []*RBACPermissionElem{
					{Key: "audit"},
				},
			},
		},
	}
	require.NoError(t, p.FillDefault(""))

	t.Run("cut_exact_should_not_cut_partial_segment_match", func(t *testing.T) {
		clone := p.Clone()
		clone.Cut(RBACPermFullKey("root.sysadmin"))

		require.NotNil(t, clone.GetElemByKey(RBACPermFullKey("root.sys")))
		require.NotNil(t, clone.GetElemByKey(RBACPermFullKey("root.sys.read")))
		require.Nil(t, clone.GetElemByKey(RBACPermFullKey("root.sysadmin")))
	})

	t.Run("cut_wildcard_should_only_remove_descendants", func(t *testing.T) {
		clone := p.Clone()
		clone.Cut(RBACPermFullKey("root.sys.*"))

		require.NotNil(t, clone.GetElemByKey(RBACPermFullKey("root.sys")))
		require.Nil(t, clone.GetElemByKey(RBACPermFullKey("root.sys.read")))
		require.Nil(t, clone.GetElemByKey(RBACPermFullKey("root.sys.write")))
		require.NotNil(t, clone.GetElemByKey(RBACPermFullKey("root.sysadmin")))
		require.NotNil(t, clone.GetElemByKey(RBACPermFullKey("root.sysadmin.audit")))
	})

	t.Run("cut_exact_deep_path_should_not_remove_ancestor", func(t *testing.T) {
		clone := p.Clone()
		clone.Cut(RBACPermFullKey("root.sys.read"))

		require.NotNil(t, clone.GetElemByKey(RBACPermFullKey("root.sys")))
		require.Nil(t, clone.GetElemByKey(RBACPermFullKey("root.sys.read")))
		require.NotNil(t, clone.GetElemByKey(RBACPermFullKey("root.sys.write")))
	})
}

func TestRBACPermissionElem_Clone(t *testing.T) {
	p := NewPermissionTree()
	p.Children = append(p.Children, &RBACPermissionElem{
		Key: "a",
	})

	p.FillDefault("")
	p2 := p.Clone()
	require.Equal(t, rbacPermissionElemKeyRoot, p2.Key)
	require.Equal(t, p.Key, p2.Key)
	require.Equal(t, p.FullKey, p2.FullKey)
	require.Equal(t, "root.a", p2.Children[0].FullKey.String())
	require.Equal(t, p.Children[0].Key, p2.Children[0].Key)
	require.Equal(t, p.Children[0].FullKey, p2.Children[0].FullKey)
}

func TestRBACPermissionElem_HasPerm(t *testing.T) {
	p := &RBACPermissionElem{
		Key: "root",
		Children: []*RBACPermissionElem{
			{
				Key: "a",
			},
			{
				Key: "b",
				Children: []*RBACPermissionElem{
					{
						Key: "c",
					},
				},
			},
		},
	}
	require.NoError(t, p.Valid())
	p.FillDefault("")

	require.True(t, p.HasPerm(RBACPermFullKey("")))
	require.True(t, p.HasPerm(RBACPermFullKey("root")))
	require.True(t, p.HasPerm(RBACPermFullKey("root.a")))
	require.True(t, p.HasPerm(RBACPermFullKey("root.b")))
	require.False(t, p.HasPerm(RBACPermFullKey("root.c")))
	require.True(t, p.HasPerm(RBACPermFullKey("root.b.c")))
	require.False(t, p.HasPerm(RBACPermFullKey("root.b.c.d")))
	require.False(t, p.HasPerm(RBACPermFullKey("root.b.c.")))

	t.Run("invalid", func(t *testing.T) {
		p.Children = append(p.Children, &RBACPermissionElem{})
		require.Error(t, p.Valid())
	})
}

func TestRBACPermissionElem_HasPerm2(t *testing.T) {
	rootPerm := &RBACPermissionElem{Key: "root"}
	require.NoError(t, rootPerm.FillDefault(""))

	rootSysPerm := &RBACPermissionElem{
		Key: "root",
		Children: []*RBACPermissionElem{
			{Key: "sys"},
		},
	}
	require.NoError(t, rootSysPerm.FillDefault(""))

	wildcardPerm := &RBACPermissionElem{
		Key: "root",
		Children: []*RBACPermissionElem{
			{
				Key: "sys",
				Children: []*RBACPermissionElem{
					{Key: "*"},
				},
			},
		},
	}
	require.NoError(t, wildcardPerm.FillDefault(""))

	leafOnlyPerm := &RBACPermissionElem{
		Key: "root",
		Children: []*RBACPermissionElem{
			{Key: "a"},
			{
				Key: "b",
				Children: []*RBACPermissionElem{
					{Key: "c"},
				},
			},
		},
	}
	require.NoError(t, leafOnlyPerm.FillDefault(""))

	var nilPerm *RBACPermissionElem

	tests := []struct {
		name     string
		p        *RBACPermissionElem
		required RBACPermFullKey
		want     bool
	}{
		{"root_require_root", rootPerm, RBACPermFullKey("root"), true},
		{"empty_require_root", &RBACPermissionElem{}, RBACPermFullKey("root"), false},
		{"root_require_empty", rootPerm, RBACPermFullKey(""), true},
		{"empty_require_empty", &RBACPermissionElem{}, RBACPermFullKey(""), true},
		{"root_sys_require_root", rootSysPerm, RBACPermFullKey("root"), false},
		{"root_require_root_sys", rootPerm, RBACPermFullKey("root.sys"), true},
		{"root_sys_require_root_sys", rootSysPerm, RBACPermFullKey("root.sys"), true},
		{"root_sys_require_descendant", rootSysPerm, RBACPermFullKey("root.sys.read"), true},
		{"wildcard_require_descendant", wildcardPerm, RBACPermFullKey("root.sys.read"), true},
		{"wildcard_require_parent", wildcardPerm, RBACPermFullKey("root.sys"), false},
		{"non_leaf_node_not_granted", leafOnlyPerm, RBACPermFullKey("root.b"), false},
		{"leaf_still_grants_descendant", leafOnlyPerm, RBACPermFullKey("root.b.c.d"), true},
		{"nil_require_non_empty", nilPerm, RBACPermFullKey("root"), false},
		{"nil_require_empty", nilPerm, RBACPermFullKey(""), true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, tt.p.HasPerm2(tt.required))
		})
	}
}
