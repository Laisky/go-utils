package utils

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestRBACUnionInvalidModeRegression checks that merging an empty valid policy
// does not resurrect children hidden by an invalid receiver grant mode.
func TestRBACUnionInvalidModeRegression(t *testing.T) {
	for _, incoming := range [][]string{nil, {"root.sys.write"}} {
		p := rbacExplicitTree("root.sys.read")
		p.Grant = "invalid"
		q := rbacExplicitTree(incoming...)
		require.False(t, p.HasPerm2("root.sys.read"))
		require.False(t, q.HasPerm2("root.sys.read"))
		p.UnionAndOverwriteBy(q)
		rbacAssertPermissions(t, p, rbacTestProbes, func(key string) bool {
			return rbacOracle(incoming, key)
		})
	}
	p, q := rbacExplicitTree("root.sys.read"), rbacExplicitTree("root.sys.write")
	q.Grant = "invalid"
	p.UnionAndOverwriteBy(q)
	rbacAssertPermissions(t, p, rbacTestProbes, func(key string) bool {
		return rbacOracle([]string{"root.sys.read"}, key)
	})
}

// TestRBACUnionMetadataRegression checks that a display merge cannot rebase a
// descendant grant through an ancestor's mismatched FullKey.
func TestRBACUnionMetadataRegression(t *testing.T) {
	p, q := rbacExplicitTree("root.sys.read"), rbacExplicitTree()
	q.Children = []*RBACPermissionElem{{Key: "sys", FullKey: "root.admin", Grant: RBACGrantNone}}
	p.Children[0].Children[0].FullKey = ""
	require.False(t, p.HasPerm2("root.admin.read"))
	require.False(t, q.HasPerm2("root.admin.read"))
	p.UnionAndOverwriteBy(q)
	require.False(t, p.HasPerm2("root.admin.read"))
	require.True(t, p.HasPerm2("root.sys.read"))
	require.Equal(t, RBACPermFullKey("root.sys"), p.Children[0].FullKey)
}

// TestRBACReciprocalUnionRegression checks opposite-direction concurrent unions
// complete and retain the union's original grants rather than deadlocking.
func TestRBACReciprocalUnionRegression(t *testing.T) {
	p, q := rbacExplicitTree("root.sys.read"), rbacExplicitTree("root.sys.write")
	start, done := make(chan struct{}), make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		<-start
		for i := 0; i < 2000; i++ {
			p.UnionAndOverwriteBy(q)
		}
	}()
	go func() {
		defer wg.Done()
		<-start
		for i := 0; i < 2000; i++ {
			q.UnionAndOverwriteBy(p)
		}
	}()
	go func() { wg.Wait(); close(done) }()
	close(start)
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		require.FailNow(t, "reciprocal union deadlocked")
	}
	for _, policy := range []*RBACPermissionElem{p, q} {
		rbacAssertPermissions(t, policy, rbacTestProbes, func(key string) bool {
			return rbacOracle([]string{"root.sys.read", "root.sys.write"}, key)
		})
	}
}

// TestRBACUnionOracle checks union on every configured grant-set pair, including
// overlapping broad and narrow grants and explicit non-granting roots.
func TestRBACUnionOracle(t *testing.T) {
	for a := uint64(0); a < 64; a++ {
		for b := uint64(0); b < 64; b++ {
			left, right := rbacMaskGrants(a), rbacMaskGrants(b)
			p, q := rbacExplicitTree(left...), rbacExplicitTree(right...)
			p.UnionAndOverwriteBy(q)
			rbacAssertPermissions(t, p, rbacTestProbes, func(key string) bool {
				return rbacOracle(left, key) || rbacOracle(right, key)
			})
			rbacAssertPermissions(t, q, rbacTestProbes, func(key string) bool {
				return rbacOracle(right, key)
			})
		}
	}
}

// TestRBACCloneBounds checks nil and cyclic inputs remain bounded and denied.
// This also protects the detached snapshots used by concurrent unions.
func TestRBACCloneBounds(t *testing.T) {
	var nilTree *RBACPermissionElem
	require.Nil(t, nilTree.Clone())
	p := &RBACPermissionElem{Key: "root"}
	p.Children = []*RBACPermissionElem{nil, p}
	clone := p.Clone()
	require.Nil(t, clone.Children[0])
	require.False(t, clone.HasPerm2("root.admin"))
	n := clone
	for depth := 0; depth <= rbacMaxDepth; depth++ {
		require.Len(t, n.Children, 2)
		n = n.Children[1]
	}
	require.Equal(t, RBACGrantNone, n.Grant)
	require.Empty(t, n.Children)
}
