package utils

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRBACPermissionElem_UnionAndOverwriteBy(t *testing.T) {
	p1 := &RBACPermissionElem{
		Key: "root",
		Children: []*RBACPermissionElem{
			{
				Key:   "a",
				Title: "a",
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
	require.NoError(t, p1.FillDefault(""))
	p2 := &RBACPermissionElem{
		Key: "root",
		Children: []*RBACPermissionElem{
			{
				Key:   "a",
				Title: "A",
			},
			{
				Key: "e",
				Children: []*RBACPermissionElem{
					{
						Key: "f",
					},
				},
			},
			{
				Key: "b",
				Children: []*RBACPermissionElem{
					{
						Key: "d",
					},
				},
			},
		},
	}
	require.NoError(t, p2.FillDefault(""))

	t.Run("union", func(t *testing.T) {
		p := p1.Clone()
		p.UnionAndOverwriteBy(p2)

		require.Equal(t, "root", p.GetElemByKey(RBACPermFullKey("root")).Key.String())
		require.Equal(t, "a", p.GetElemByKey(RBACPermFullKey("root.a")).Key.String())
		require.Equal(t, "b", p.GetElemByKey(RBACPermFullKey("root.b")).Key.String())
		require.Equal(t, "c", p.GetElemByKey(RBACPermFullKey("root.b.c")).Key.String())
		require.Equal(t, "e", p.GetElemByKey(RBACPermFullKey("root.e")).Key.String())
		require.Equal(t, "f", p.GetElemByKey(RBACPermFullKey("root.e.f")).Key.String())
		require.Nil(t, p.GetElemByKey(RBACPermFullKey("root.e.t")))
	})

	t.Run("intersection", func(t *testing.T) {
		p := p1.Clone()
		p.Intersection(p2)

		require.Equal(t, "root", p.GetElemByKey(RBACPermFullKey("root")).Key.String())
		require.Equal(t, "a", p.GetElemByKey(RBACPermFullKey("root.a")).Key.String())
		require.Equal(t, "b", p.GetElemByKey(RBACPermFullKey("root.b")).Key.String())
		require.Nil(t, p.GetElemByKey(RBACPermFullKey("root.b.c")))
		require.Nil(t, p.GetElemByKey(RBACPermFullKey("root.e")))
		require.Nil(t, p.GetElemByKey(RBACPermFullKey("root.e.f")))
		require.Nil(t, p.GetElemByKey(RBACPermFullKey("root.e.t")))
	})

	t.Run("overwrite without intercetion", func(t *testing.T) {
		p := p1.Clone()
		p.OverwriteBy(p2, false)

		require.Equal(t, "root", p.GetElemByKey(RBACPermFullKey("root")).Key.String())
		require.Equal(t, "a", p.GetElemByKey(RBACPermFullKey("root.a")).Key.String())
		require.Equal(t, "A", p.GetElemByKey(RBACPermFullKey("root.a")).Title)
		require.Equal(t, "b", p.GetElemByKey(RBACPermFullKey("root.b")).Key.String())
		require.Equal(t, "c", p.GetElemByKey(RBACPermFullKey("root.b.c")).Key.String())
		require.Nil(t, p.GetElemByKey(RBACPermFullKey("root.e")))
		require.Nil(t, p.GetElemByKey(RBACPermFullKey("root.e.f")))
		require.Nil(t, p.GetElemByKey(RBACPermFullKey("root.e.t")))
	})

	t.Run("overwrite with intercetion", func(t *testing.T) {
		p := p1.Clone()
		p.OverwriteBy(p2, true)

		require.Equal(t, "root", p.GetElemByKey(RBACPermFullKey("root")).Key.String())
		require.Equal(t, "a", p.GetElemByKey(RBACPermFullKey("root.a")).Key.String())
		require.Equal(t, "A", p.GetElemByKey(RBACPermFullKey("root.a")).Title)
		require.Equal(t, "b", p.GetElemByKey(RBACPermFullKey("root.b")).Key.String())
		require.Nil(t, p.GetElemByKey(RBACPermFullKey("root.b.c")))
		require.Nil(t, p.GetElemByKey(RBACPermFullKey("root.e")))
		require.Nil(t, p.GetElemByKey(RBACPermFullKey("root.e.f")))
		require.Nil(t, p.GetElemByKey(RBACPermFullKey("root.e.t")))
	})

	t.Run("cut", func(t *testing.T) {
		p := p1.Clone()
		p.UnionAndOverwriteBy(p2)

		p.Cut("root.b")
		require.Equal(t, "root", p.GetElemByKey(RBACPermFullKey("root")).Key.String())
		require.Equal(t, "a", p.GetElemByKey(RBACPermFullKey("root.a")).Key.String())
		require.Nil(t, p.GetElemByKey(RBACPermFullKey("root.b")))
		require.Nil(t, p.GetElemByKey(RBACPermFullKey("root.b.c")))
		require.Equal(t, "e", p.GetElemByKey(RBACPermFullKey("root.e")).Key.String())
		require.Equal(t, "f", p.GetElemByKey(RBACPermFullKey("root.e.f")).Key.String())
		require.Nil(t, p.GetElemByKey(RBACPermFullKey("root.e.t")))
	})
}

func TestRBACPermissionElem_ComplexScenarios(t *testing.T) {
	t.Run("deep nested permissions", func(t *testing.T) {
		p := &RBACPermissionElem{
			Key: "root",
			Children: []*RBACPermissionElem{
				{
					Key: "admin",
					Children: []*RBACPermissionElem{
						{
							Key: "users",
							Children: []*RBACPermissionElem{
								{Key: "create"},
								{Key: "delete"},
								{Key: "update"},
							},
						},
						{
							Key: "settings",
							Children: []*RBACPermissionElem{
								{Key: "read"},
								{Key: "write"},
							},
						},
					},
				},
			},
		}
		require.NoError(t, p.FillDefault(""))

		// Test deep permission checks
		require.True(t, p.HasPerm(RBACPermFullKey("root.admin.users.create")))
		require.True(t, p.HasPerm(RBACPermFullKey("root.admin.settings.write")))
		require.False(t, p.HasPerm(RBACPermFullKey("root.admin.users.invalid")))

		// Test parent permissions
		require.True(t, p.HasPerm(RBACPermFullKey("root.admin")))
		require.True(t, p.HasPerm(RBACPermFullKey("root.admin.users")))

		// Test non-existent paths
		require.False(t, p.HasPerm(RBACPermFullKey("invalid")))
		require.False(t, p.HasPerm(RBACPermFullKey("root.invalid")))
		require.False(t, p.HasPerm(RBACPermFullKey("root.admin.users.create.invalid")))
	})

	t.Run("complex union operations", func(t *testing.T) {
		base := &RBACPermissionElem{
			Key: "root",
			Children: []*RBACPermissionElem{
				{
					Key:   "projects",
					Title: "Projects",
					Children: []*RBACPermissionElem{
						{
							Key:   "view",
							Title: "View Projects",
						},
					},
				},
			},
		}
		require.NoError(t, base.FillDefault(""))

		additional := &RBACPermissionElem{
			Key: "root",
			Children: []*RBACPermissionElem{
				{
					Key:   "projects",
					Title: "Updated Projects",
					Children: []*RBACPermissionElem{
						{
							Key:   "view",
							Title: "View All Projects",
						},
						{
							Key:   "edit",
							Title: "Edit Projects",
						},
					},
				},
				{
					Key:   "users",
					Title: "Users Management",
				},
			},
		}
		require.NoError(t, additional.FillDefault(""))

		// Test union
		baseClone := base.Clone()
		baseClone.UnionAndOverwriteBy(additional)

		// Verify structure after union
		projectsNode := baseClone.GetElemByKey(RBACPermFullKey("root.projects"))
		require.NotNil(t, projectsNode)
		require.Equal(t, "Updated Projects", projectsNode.Title)
		require.Equal(t, 2, len(projectsNode.Children))

		// Verify new nodes were added
		usersNode := baseClone.GetElemByKey(RBACPermFullKey("root.users"))
		require.NotNil(t, usersNode)
		require.Equal(t, "Users Management", usersNode.Title)
	})

	t.Run("multiple operations sequence", func(t *testing.T) {
		p1 := &RBACPermissionElem{
			Key: "root",
			Children: []*RBACPermissionElem{
				{
					Key:   "finance",
					Title: "Finance",
					Children: []*RBACPermissionElem{
						{Key: "view"},
						{Key: "edit"},
					},
				},
				{
					Key:   "hr",
					Title: "Human Resources",
					Children: []*RBACPermissionElem{
						{Key: "employees"},
					},
				},
			},
		}
		require.NoError(t, p1.FillDefault(""))

		p2 := &RBACPermissionElem{
			Key: "root",
			Children: []*RBACPermissionElem{
				{
					Key:   "finance",
					Title: "Financial Department",
					Children: []*RBACPermissionElem{
						{Key: "view"},
						{Key: "reports"},
					},
				},
				{
					Key:   "it",
					Title: "IT Department",
					Children: []*RBACPermissionElem{
						{Key: "servers"},
					},
				},
			},
		}
		require.NoError(t, p2.FillDefault(""))

		// Multiple operations sequence
		p := p1.Clone()

		// First union with p2
		p.UnionAndOverwriteBy(p2)
		require.Equal(t, "Financial Department", p.GetElemByKey(RBACPermFullKey("root.finance")).Title)
		require.NotNil(t, p.GetElemByKey(RBACPermFullKey("root.finance.edit")))
		require.NotNil(t, p.GetElemByKey(RBACPermFullKey("root.finance.reports")))
		require.NotNil(t, p.GetElemByKey(RBACPermFullKey("root.it")))
		require.NotNil(t, p.GetElemByKey(RBACPermFullKey("root.hr")))

		// Then cut HR
		p.Cut(RBACPermFullKey("root.hr"))
		require.Nil(t, p.GetElemByKey(RBACPermFullKey("root.hr")))
		require.NotNil(t, p.GetElemByKey(RBACPermFullKey("root.finance")))
		require.NotNil(t, p.GetElemByKey(RBACPermFullKey("root.it")))

		// Add IT security through another union
		p3 := &RBACPermissionElem{
			Key: "root",
			Children: []*RBACPermissionElem{
				{
					Key: "it",
					Children: []*RBACPermissionElem{
						{Key: "security"},
					},
				},
			},
		}
		require.NoError(t, p3.FillDefault(""))

		p.UnionAndOverwriteBy(p3)
		require.NotNil(t, p.GetElemByKey(RBACPermFullKey("root.it.servers")))
		require.NotNil(t, p.GetElemByKey(RBACPermFullKey("root.it.security")))
	})

	t.Run("edge cases", func(t *testing.T) {
		// Test with empty tree
		emptyTree := NewPermissionTree()
		emptyTree.Cut(RBACPermFullKey("any.key"))
		require.Equal(t, rbacPermissionElemKeyRoot, emptyTree.Key)
		require.Empty(t, emptyTree.Children)

		// Test with root key
		rootTree := NewPermissionTree()
		rootTree.Children = append(rootTree.Children, &RBACPermissionElem{Key: "child"})
		require.NoError(t, rootTree.FillDefault(""))

		// Cutting root preserves the receiver object, not its authority.
		require.True(t, rootTree.HasPerm2("root.child"))
		rootTree.Cut(RBACPermFullKey("root"))
		require.NotNil(t, rootTree)
		require.Equal(t, rbacPermissionElemKeyRoot, rootTree.Key)
		require.Empty(t, rootTree.Children)
		require.Equal(t, RBACGrantNone, rootTree.Grant)
		require.False(t, rootTree.HasPerm2("root"))
		require.False(t, rootTree.HasPerm2("root.child"))
		require.False(t, rootTree.HasPerm2("root.admin"))

		// Test with non-existent key
		p := &RBACPermissionElem{
			Key: "root",
			Children: []*RBACPermissionElem{
				{Key: "a"},
			},
		}
		require.NoError(t, p.FillDefault(""))
		p.Cut(RBACPermFullKey("root.nonexistent"))
		require.NotNil(t, p.GetElemByKey(RBACPermFullKey("root.a")))
	})
}
