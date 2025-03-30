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

func TestRBACPermissionElemFullKey_Contains(t *testing.T) {
	type args struct {
		acquire RBACPermFullKey
	}
	tests := []struct {
		name string
		p    RBACPermFullKey
		args args
		want bool
	}{
		{"0", RBACPermFullKey("a.b"), args{RBACPermFullKey("a")}, true},
		{"1", RBACPermFullKey("a.b"), args{RBACPermFullKey("b")}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.p.Contains(tt.args.acquire); got != tt.want {
				t.Errorf("RBACPermissionElemFullKey.Contains() = %v, want %v", got, tt.want)
			}
		})
	}
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

		// Cutting root shouldn't do anything
		rootTree.Cut(RBACPermFullKey("root"))
		require.NotNil(t, rootTree)
		require.Equal(t, 1, len(rootTree.Children))

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

func TestRBACPermissionElem_ValueScan(t *testing.T) {
	// Create a permission tree for testing
	p := &RBACPermissionElem{
		Key:   "test",
		Title: "Test Permission",
		Children: []*RBACPermissionElem{
			{
				Key:   "child1",
				Title: "Child 1",
			},
			{
				Key:   "child2",
				Title: "Child 2",
				Children: []*RBACPermissionElem{
					{
						Key:   "grandchild",
						Title: "Grand Child",
					},
				},
			},
		},
	}
	require.NoError(t, p.FillDefault(""))

	// Test Value() method
	t.Run("Value", func(t *testing.T) {
		val, err := p.Value()
		require.NoError(t, err)
		require.NotNil(t, val)

		// Verify the result is a string
		jsonStr, ok := val.(string)
		require.True(t, ok)

		// Verify the JSON contains expected fields
		require.Contains(t, jsonStr, `"key":"test"`)
		require.Contains(t, jsonStr, `"title":"Test Permission"`)
		require.Contains(t, jsonStr, `"child1"`)
		require.Contains(t, jsonStr, `"child2"`)
		require.Contains(t, jsonStr, `"grandchild"`)
	})

	// Test Scan() method
	t.Run("Scan", func(t *testing.T) {
		// Get JSON bytes from Value method
		val, err := p.Value()
		require.NoError(t, err)
		jsonBytes := []byte(val.(string))

		// Create a new struct to scan into
		scanned := &RBACPermissionElem{}

		// Scan the JSON into the new struct
		err = scanned.Scan(jsonBytes)
		require.NoError(t, err)

		// Verify struct was properly scanned
		require.Equal(t, p.Key, scanned.Key)
		require.Equal(t, p.Title, scanned.Title)
		require.Equal(t, 2, len(scanned.Children))
		require.Equal(t, "child1", scanned.Children[0].Key.String())
		require.Equal(t, "child2", scanned.Children[1].Key.String())
		require.Equal(t, 1, len(scanned.Children[1].Children))
		require.Equal(t, "grandchild", scanned.Children[1].Children[0].Key.String())
	})

	// Test Scan() with invalid input
	t.Run("Scan_InvalidInput", func(t *testing.T) {
		scanned := &RBACPermissionElem{}

		// Test with malformed JSON
		err := scanned.Scan([]byte(`{"key": "test", "children": [{"key": `))
		require.Error(t, err)

		// Test with wrong JSON structure
		err = scanned.Scan([]byte(`{"wrong_field": "value"}`))
		require.NoError(t, err) // Should not error, just create empty struct
		require.Empty(t, scanned.Key)
	})

	// Test round-trip conversion
	t.Run("ValueScan_RoundTrip", func(t *testing.T) {
		// Complex permission tree with filled defaults
		original := &RBACPermissionElem{
			Key:   "complex",
			Title: "Complex Tree",
			Children: []*RBACPermissionElem{
				{
					Key:   "level1",
					Title: "Level 1",
					Children: []*RBACPermissionElem{
						{
							Key:   "level2a",
							Title: "Level 2A",
						},
						{
							Key:   "level2b",
							Title: "Level 2B",
						},
					},
				},
			},
		}
		require.NoError(t, original.FillDefault(""))

		// Convert to database value
		val, err := original.Value()
		require.NoError(t, err)

		// Scan back into a new struct
		scanned := &RBACPermissionElem{}
		err = scanned.Scan([]byte(val.(string)))
		require.NoError(t, err)

		// Must manually fill default values since Scan doesn't do this
		require.NoError(t, scanned.FillDefault(""))

		// Verify that field values match
		require.Equal(t, original.Key, scanned.Key)
		require.Equal(t, original.Title, scanned.Title)
		require.Equal(t, len(original.Children), len(scanned.Children))
		require.Equal(t, original.Children[0].Key, scanned.Children[0].Key)
		require.Equal(t, original.Children[0].Title, scanned.Children[0].Title)
		require.Equal(t, len(original.Children[0].Children), len(scanned.Children[0].Children))

		// Check full keys
		require.Equal(t, "complex", scanned.FullKey.String())
		require.Equal(t, "complex.level1", scanned.Children[0].FullKey.String())
		require.Equal(t, "complex.level1.level2a", scanned.Children[0].Children[0].FullKey.String())
	})
}
