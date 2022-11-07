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
