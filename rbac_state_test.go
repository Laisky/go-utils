package utils

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

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

	// Test Scan() method with []byte
	t.Run("Scan_Bytes", func(t *testing.T) {
		val, err := p.Value()
		require.NoError(t, err)
		jsonBytes := []byte(val.(string))

		scanned := &RBACPermissionElem{}
		err = scanned.Scan(jsonBytes)
		require.NoError(t, err)

		require.Equal(t, p.Key, scanned.Key)
		require.Equal(t, p.Title, scanned.Title)
		require.Equal(t, 2, len(scanned.Children))
		require.Equal(t, "child1", scanned.Children[0].Key.String())
		require.Equal(t, "child2", scanned.Children[1].Key.String())
		require.Equal(t, 1, len(scanned.Children[1].Children))
		require.Equal(t, "grandchild", scanned.Children[1].Children[0].Key.String())
	})

	// Test Scan() method with string (from some SQL drivers)
	t.Run("Scan_String", func(t *testing.T) {
		val, err := p.Value()
		require.NoError(t, err)
		jsonStr := val.(string)

		scanned := &RBACPermissionElem{}
		err = scanned.Scan(jsonStr)
		require.NoError(t, err)

		require.Equal(t, p.Key, scanned.Key)
		require.Equal(t, p.Title, scanned.Title)
		require.Equal(t, 2, len(scanned.Children))
	})

	// Test Scan() with nil input
	t.Run("Scan_Nil", func(t *testing.T) {
		scanned := &RBACPermissionElem{}
		err := scanned.Scan(nil)
		require.Error(t, err)
		require.Contains(t, err.Error(), "nil")
	})

	// Test Scan() with unsupported type
	t.Run("Scan_UnsupportedType", func(t *testing.T) {
		scanned := &RBACPermissionElem{}
		err := scanned.Scan(12345)
		require.Error(t, err)
		require.Contains(t, err.Error(), "unsupported type")
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

		val, err := original.Value()
		require.NoError(t, err)

		scanned := &RBACPermissionElem{}
		err = scanned.Scan([]byte(val.(string)))
		require.NoError(t, err)
		require.NoError(t, scanned.FillDefault(""))

		require.Equal(t, original.Key, scanned.Key)
		require.Equal(t, original.Title, scanned.Title)
		require.Equal(t, len(original.Children), len(scanned.Children))
		require.Equal(t, original.Children[0].Key, scanned.Children[0].Key)
		require.Equal(t, original.Children[0].Title, scanned.Children[0].Title)
		require.Equal(t, len(original.Children[0].Children), len(scanned.Children[0].Children))

		require.Equal(t, "complex", scanned.FullKey.String())
		require.Equal(t, "complex.level1", scanned.Children[0].FullKey.String())
		require.Equal(t, "complex.level1.level2a", scanned.Children[0].Children[0].FullKey.String())
	})
}

// TestRBACPermissionElem_Valid_NoSideEffect verifies that Valid() does not mutate the tree.
func TestRBACPermissionElem_Valid_NoSideEffect(t *testing.T) {
	p := &RBACPermissionElem{
		Key:      "root",
		Children: nil, // explicitly nil
	}

	require.NoError(t, p.Valid())
	// Valid() should NOT have changed nil Children to []
	require.Nil(t, p.Children)
}

// TestRBACPermissionElem_Valid_EmptyKey verifies Valid() reports empty key at different levels.
func TestRBACPermissionElem_Valid_EmptyKey(t *testing.T) {
	t.Run("root_empty_key", func(t *testing.T) {
		p := &RBACPermissionElem{}
		require.Error(t, p.Valid())
	})

	t.Run("child_empty_key", func(t *testing.T) {
		p := &RBACPermissionElem{
			Key: "root",
			Children: []*RBACPermissionElem{
				{Key: "a"},
				{Key: ""},
			},
		}
		require.Error(t, p.Valid())
	})
}

// TestRBACPermissionElem_UnionAndOverwriteBy_NoPointerSharing verifies that
// UnionAndOverwriteBy clones new nodes so that mutating the result does not
// affect the source tree.
func TestRBACPermissionElem_UnionAndOverwriteBy_NoPointerSharing(t *testing.T) {
	p1 := &RBACPermissionElem{
		Key: "root",
		Children: []*RBACPermissionElem{
			{Key: "a"},
		},
	}
	require.NoError(t, p1.FillDefault(""))

	p2 := &RBACPermissionElem{
		Key: "root",
		Children: []*RBACPermissionElem{
			{Key: "b", Title: "Original B"},
		},
	}
	require.NoError(t, p2.FillDefault(""))

	p1.UnionAndOverwriteBy(p2)

	// Mutate the merged result's "b" node title
	bNode := p1.GetElemByKey(RBACPermFullKey("root.b"))
	require.NotNil(t, bNode)
	bNode.Title = "Modified B"

	// The original p2 tree should be unaffected
	originalB := p2.GetElemByKey(RBACPermFullKey("root.b"))
	require.NotNil(t, originalB)
	require.Equal(t, "Original B", originalB.Title)
}

// TestRBACPermissionElem_UnionAndOverwriteBy_MismatchedRootKey verifies
// that union with different root keys is a no-op.
func TestRBACPermissionElem_UnionAndOverwriteBy_MismatchedRootKey(t *testing.T) {
	p1 := &RBACPermissionElem{Key: "root", Children: []*RBACPermissionElem{{Key: "a"}}}
	require.NoError(t, p1.FillDefault(""))

	p2 := &RBACPermissionElem{Key: "other", Children: []*RBACPermissionElem{{Key: "b"}}}
	require.NoError(t, p2.FillDefault(""))

	p1.UnionAndOverwriteBy(p2)
	// p1 should be unchanged
	require.NotNil(t, p1.GetElemByKey(RBACPermFullKey("root.a")))
	require.Nil(t, p1.GetElemByKey(RBACPermFullKey("root.b")))
}

// TestRBACPermissionElem_Intersection_MismatchedRootKey verifies disjoint roots
// produce an empty permission set, never the unchanged receiver grant set.
func TestRBACPermissionElem_Intersection_MismatchedRootKey(t *testing.T) {
	p1 := &RBACPermissionElem{Key: "root", Children: []*RBACPermissionElem{{Key: "a"}}}
	require.NoError(t, p1.FillDefault(""))

	p2 := &RBACPermissionElem{Key: "other"}
	require.True(t, p1.HasPerm2("root.a"))
	require.False(t, p2.HasPerm2("root.a"))
	p1.Intersection(p2)
	require.Nil(t, p1.GetElemByKey(RBACPermFullKey("root.a")))
	require.Equal(t, RBACGrantNone, p1.Grant)
	for _, key := range []RBACPermFullKey{"root", "root.a", "root.admin", "other", "other.admin"} {
		require.False(t, p1.HasPerm2(key), "intersection must deny %s", key)
	}
	require.True(t, p2.HasPerm2("other.admin"), "other input must remain unchanged")
}

// TestRBACPermissionElem_OverwriteBy_MismatchedRootKey distinguishes metadata
// overwrite (no change) from intersection (an empty permission set).
func TestRBACPermissionElem_OverwriteBy_MismatchedRootKey(t *testing.T) {
	p1 := &RBACPermissionElem{Key: "root", Children: []*RBACPermissionElem{{Key: "a"}}}
	require.NoError(t, p1.FillDefault(""))

	p2 := &RBACPermissionElem{Key: "other"}
	p1.OverwriteBy(p2, false)
	require.NotNil(t, p1.GetElemByKey(RBACPermFullKey("root.a")))
	require.True(t, p1.HasPerm2("root.a"))
	require.False(t, p2.HasPerm2("root.a"))

	p1.OverwriteBy(p2, true)
	require.Nil(t, p1.GetElemByKey(RBACPermFullKey("root.a")))
	require.Equal(t, RBACGrantNone, p1.Grant)
	for _, key := range []RBACPermFullKey{"root", "root.a", "root.admin", "other", "other.admin"} {
		require.False(t, p1.HasPerm2(key), "intersection must deny %s", key)
	}
	require.True(t, p2.HasPerm2("other.admin"), "other input must remain unchanged")
}

// TestRBACPermissionElem_GetElemByKey_EdgeCases tests edge cases of GetElemByKey.
func TestRBACPermissionElem_GetElemByKey_EdgeCases(t *testing.T) {
	p := &RBACPermissionElem{
		Key:      "root",
		Children: []*RBACPermissionElem{},
	}
	require.NoError(t, p.FillDefault(""))

	require.Nil(t, p.GetElemByKey(RBACPermFullKey("")))
	require.Nil(t, p.GetElemByKey(RBACPermFullKey("nonexistent")))
	require.NotNil(t, p.GetElemByKey(RBACPermFullKey("root")))

	emptyKeyElem := &RBACPermissionElem{}
	require.Nil(t, emptyKeyElem.GetElemByKey(RBACPermFullKey("anything")))
}

// TestRBACPermissionElem_FillDefault_EmptyKey tests that FillDefault rejects empty keys.
func TestRBACPermissionElem_FillDefault_EmptyKey(t *testing.T) {
	p := &RBACPermissionElem{}
	require.Error(t, p.FillDefault(""))
}

// TestRBACPermissionElem_FillDefault_NilChildren tests that FillDefault initializes nil children.
func TestRBACPermissionElem_FillDefault_NilChildren(t *testing.T) {
	p := &RBACPermissionElem{
		Key:      "root",
		Children: nil,
	}
	require.NoError(t, p.FillDefault(""))
	require.NotNil(t, p.Children)
	require.Empty(t, p.Children)
}

// TestRBACPermissionElem_Clone_DeepIndependence verifies Clone creates fully
// independent copies at all levels.
func TestRBACPermissionElem_Clone_DeepIndependence(t *testing.T) {
	p := &RBACPermissionElem{
		Key: "root",
		Children: []*RBACPermissionElem{
			{
				Key: "a",
				Children: []*RBACPermissionElem{
					{Key: "b", Title: "Original"},
				},
			},
		},
	}
	require.NoError(t, p.FillDefault(""))

	clone := p.Clone()
	clone.GetElemByKey(RBACPermFullKey("root.a.b")).Title = "Modified"

	require.Equal(t, "Original", p.GetElemByKey(RBACPermFullKey("root.a.b")).Title)
	require.Equal(t, "Modified", clone.GetElemByKey(RBACPermFullKey("root.a.b")).Title)
}

// TestRBACPermissionElem_Concurrent_Reads verifies that concurrent reads
// on the same tree don't race.
func TestRBACPermissionElem_Concurrent_Reads(t *testing.T) {
	p := &RBACPermissionElem{
		Key: "root",
		Children: []*RBACPermissionElem{
			{
				Key: "admin",
				Children: []*RBACPermissionElem{
					{Key: "read"},
					{Key: "write"},
				},
			},
			{Key: "user"},
		},
	}
	require.NoError(t, p.FillDefault(""))

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(4)
		go func() {
			defer wg.Done()
			p.HasPerm(RBACPermFullKey("root.admin.read"))
		}()
		go func() {
			defer wg.Done()
			p.HasPerm2(RBACPermFullKey("root.admin.read"))
		}()
		go func() {
			defer wg.Done()
			p.GetElemByKey(RBACPermFullKey("root.admin"))
		}()
		go func() {
			defer wg.Done()
			p.Clone()
		}()
	}
	wg.Wait()
}

// TestRBACPermissionElem_Concurrent_ReadWrite verifies that concurrent reads
// and writes on the same tree are safe.
func TestRBACPermissionElem_Concurrent_ReadWrite(t *testing.T) {
	p := &RBACPermissionElem{
		Key: "root",
		Children: []*RBACPermissionElem{
			{
				Key: "admin",
				Children: []*RBACPermissionElem{
					{Key: "read"},
					{Key: "write"},
					{Key: "delete"},
				},
			},
			{Key: "user"},
			{Key: "guest"},
		},
	}
	require.NoError(t, p.FillDefault(""))

	var wg sync.WaitGroup

	// Concurrent readers
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			p.HasPerm(RBACPermFullKey("root.admin.read"))
			p.HasPerm2(RBACPermFullKey("root.admin.write"))
			p.GetElemByKey(RBACPermFullKey("root.user"))
			p.Valid()
		}()
	}

	// Concurrent writers: Cut, then re-merge
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			other := &RBACPermissionElem{
				Key: "root",
				Children: []*RBACPermissionElem{
					{Key: "extra"},
				},
			}
			_ = other.FillDefault("")
			p.UnionAndOverwriteBy(other)
		}()
	}

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			p.Cut(RBACPermFullKey("root.extra"))
		}()
	}

	wg.Wait()

	// Tree should still be valid (basic structural integrity)
	require.NotNil(t, p.GetElemByKey(RBACPermFullKey("root")))
	require.NotNil(t, p.GetElemByKey(RBACPermFullKey("root.admin")))
}

// TestRBACPermissionElem_Cut_EmptyKey verifies Cut with empty key is a no-op.
func TestRBACPermissionElem_Cut_EmptyKey(t *testing.T) {
	p := &RBACPermissionElem{
		Key:      "root",
		Children: []*RBACPermissionElem{{Key: "a"}},
	}
	require.NoError(t, p.FillDefault(""))

	p.Cut(RBACPermFullKey(""))
	require.NotNil(t, p.GetElemByKey(RBACPermFullKey("root.a")))
}

// TestRBACPermissionElem_HasPerm_EmptyKey verifies HasPerm with empty-key tree.
func TestRBACPermissionElem_HasPerm_EmptyKeyTree(t *testing.T) {
	p := &RBACPermissionElem{}
	require.True(t, p.HasPerm(RBACPermFullKey("")))
	require.False(t, p.HasPerm(RBACPermFullKey("root")))
}

// TestRBACPermissionElem_HasPerm2_EmptyKeyTree verifies HasPerm2 with empty-key tree.
func TestRBACPermissionElem_HasPerm2_EmptyKeyTree(t *testing.T) {
	p := &RBACPermissionElem{}
	require.True(t, p.HasPerm2(RBACPermFullKey("")))
	require.False(t, p.HasPerm2(RBACPermFullKey("root")))
}

// TestRBACPermissionElem_FillDefault_ChildError tests FillDefault propagates child errors.
func TestRBACPermissionElem_FillDefault_ChildError(t *testing.T) {
	p := &RBACPermissionElem{
		Key: "root",
		Children: []*RBACPermissionElem{
			{Key: "valid"},
			{Key: ""}, // invalid
		},
	}
	err := p.FillDefault("")
	require.Error(t, err)
	require.Contains(t, err.Error(), "key is empty")
}

// TestRBACPermissionElem_Scan_RoundTrip_String tests round-trip via string scan.
func TestRBACPermissionElem_Scan_RoundTrip_String(t *testing.T) {
	original := &RBACPermissionElem{
		Key:   "root",
		Title: "Root",
		Children: []*RBACPermissionElem{
			{Key: "a", Title: "A"},
		},
	}
	require.NoError(t, original.FillDefault(""))

	val, err := original.Value()
	require.NoError(t, err)

	// Scan from string (simulating PostgreSQL driver behavior)
	scanned := &RBACPermissionElem{}
	err = scanned.Scan(val.(string))
	require.NoError(t, err)
	require.Equal(t, original.Key, scanned.Key)
	require.Equal(t, original.Title, scanned.Title)
	require.Equal(t, 1, len(scanned.Children))
}
