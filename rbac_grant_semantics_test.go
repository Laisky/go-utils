package utils

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// rbacOracle reports whether grants cover required using an independent
// segment-based model; it never calls the production permission matchers.
func rbacOracle(grants []string, required string) bool {
	if required == "" {
		return true
	}
	r := strings.Split(required, ".")
	for _, g := range grants {
		parts := strings.Split(g, ".")
		wildcard := len(parts) > 1 && parts[len(parts)-1] == "*"
		if wildcard {
			parts = parts[:len(parts)-1]
		}
		if len(r) < len(parts) || (wildcard && len(r) == len(parts)) {
			continue
		}
		same := true
		for i, part := range parts {
			if r[i] != part {
				same = false
				break
			}
		}
		if same {
			return true
		}
	}
	return false
}

// rbacExplicitTree returns a canonical explicit tree for test grant paths.
// A path outside root or an invalid fixture panics instead of building bad input.
func rbacExplicitTree(grants ...string) *RBACPermissionElem {
	p := &RBACPermissionElem{Key: "root", Grant: RBACGrantNone}
	for _, g := range grants {
		parts := strings.Split(g, ".")
		if parts[0] != "root" {
			panic("test grant outside root")
		}
		n := p
		for _, part := range parts[1:] {
			var next *RBACPermissionElem
			for _, c := range n.Children {
				if c.Key == RBACPermKey(part) {
					next = c
					break
				}
			}
			if next == nil {
				next = &RBACPermissionElem{Key: RBACPermKey(part), Grant: RBACGrantNone}
				n.Children = append(n.Children, next)
			}
			n = next
		}
		n.Grant = RBACGrantSubtree
	}
	if err := p.FillDefault(""); err != nil {
		panic(err)
	}
	return p
}

// rbacAssertPermissions asserts each probe against want using tree p.
// It reports failures through t and does not change the policy.
func rbacAssertPermissions(t testing.TB, p *RBACPermissionElem, probes []string, want func(string) bool) {
	t.Helper()
	for _, q := range probes {
		require.Equalf(t, want(q), p.HasPerm2(RBACPermFullKey(q)), "permission %q", q)
	}
}

var rbacTestGrantKeys = []string{"root", "root.sys", "root.sys.*", "root.sys.read", "root.sys.write", "root.sysadmin.audit"}
var rbacTestProbes = []string{"", "root", "root.sys", "root.sys.*", "root.sys.read", "root.sys.read.child", "root.sys.write", "root.sys.admin", "root.sys.future.action", "root.sysadmin", "root.sysadmin.audit", "root.sysadmin.audit.child", "root.data", "root.data.audit", "root2", "other.root"}

// rbacMaskGrants returns grant keys selected by the low bits of mask.
func rbacMaskGrants(mask uint64) []string {
	var out []string
	for i, key := range rbacTestGrantKeys {
		if mask&(1<<i) != 0 {
			out = append(out, key)
		}
	}
	return out
}

// TestRBACSemanticIntersectionExhaustive checks both intersection APIs
// against the independent oracle for every pair of configured grant sets.
func TestRBACSemanticIntersectionExhaustive(t *testing.T) {
	for a := uint64(0); a < 64; a++ {
		for b := uint64(0); b < 64; b++ {
			left, right := rbacMaskGrants(a), rbacMaskGrants(b)
			for _, overwrite := range []bool{false, true} {
				p, q := rbacExplicitTree(left...), rbacExplicitTree(right...)
				before, err := json.Marshal(q)
				require.NoError(t, err)
				if overwrite {
					p.OverwriteBy(q, true)
				} else {
					p.Intersection(q)
				}
				for _, probe := range rbacTestProbes {
					want := rbacOracle(left, probe) && rbacOracle(right, probe)
					require.Equalf(t, want, p.HasPerm2(RBACPermFullKey(probe)),
						"a=%d b=%d overwrite=%v query=%q", a, b, overwrite, probe)
				}
				after, err := json.Marshal(q)
				require.NoError(t, err)
				require.Equal(t, before, after, "other input was mutated")
			}
		}
	}
}

// TestRBACRestrictionPersistence checks revocation across clone, JSON,
// SQL serialization, reinitialization, and subsequent metadata operations.
func TestRBACRestrictionPersistence(t *testing.T) {
	for _, op := range []string{"cut", "intersection", "overwrite"} {
		t.Run(op, func(t *testing.T) {
			p := rbacRegressionTree("read")
			switch op {
			case "cut":
				p.Cut("root.sys.read")
			case "intersection":
				p.Intersection(rbacRegressionTree("write"))
			case "overwrite":
				p.OverwriteBy(rbacRegressionTree("write"), true)
			}
			b, err := json.Marshal(p)
			require.NoError(t, err)
			require.Contains(t, string(b), `"grant":"none"`, "non-grant state not persisted")
			var fromJSON RBACPermissionElem
			require.NoError(t, json.Unmarshal(b, &fromJSON))
			value, err := p.Value()
			require.NoError(t, err)
			var fromSQL RBACPermissionElem
			require.NoError(t, fromSQL.Scan(value))
			for name, copy := range map[string]*RBACPermissionElem{"live": p, "clone": p.Clone(), "json": &fromJSON, "sql": &fromSQL} {
				require.NoError(t, copy.FillDefault(""))
				rbacAssertPermissions(t, copy, rbacTestProbes, func(q string) bool { return q == "" })
				sys := copy.GetElemByKey("root.sys")
				require.NotNilf(t, sys, "%s lost common structural node", name)
				require.Falsef(t, sys.HasPerm2("root.sys.admin"), "%s subtree regranted admin", name)
				// Subsequent metadata operations must not resurrect the emptied parent.
				copy.OverwriteBy(rbacRegressionTree("admin"), false)
				rbacAssertPermissions(t, copy, rbacTestProbes, func(q string) bool { return q == "" })
			}
		})
	}
}

// TestRBACCutMonotonicity checks that every configured cut both revokes
// its target and never grants a permission absent from the original tree.
func TestRBACCutMonotonicity(t *testing.T) {
	targets := []string{"root", "root.*", "*", "root.sys", "root.sys.*", "root.sys.read", "root.sys.read.child", "root.sysadmin.audit", "root.data.audit", "other.root"}
	for mask := uint64(0); mask < 64; mask++ {
		for _, target := range targets {
			grants := rbacMaskGrants(mask)
			p := rbacExplicitTree(grants...)
			p.Cut(RBACPermFullKey(target))
			effectiveTarget := target
			if target == "*" {
				effectiveTarget = "root.*"
			}
			for _, probe := range rbacTestProbes {
				got := p.HasPerm2(RBACPermFullKey(probe))
				require.Falsef(t, got && !rbacOracle(grants, probe), "cut %q grants new %q for %v", target, probe, grants)
				require.Falsef(t, probe != "" && got && rbacOracle([]string{effectiveTarget}, probe), "cut %q failed to revoke %q for %v", target, probe, grants)
			}
		}
	}
}

// TestRBACLegacyAndDefaultState checks explicit denial defaults and
// backward-compatible interpretation of uninitialized legacy policies.
func TestRBACLegacyAndDefaultState(t *testing.T) {
	require.False(t, NewPermissionTree().HasPerm2("root.admin"), "new empty tree grants admin")
	for _, fill := range []bool{false, true} {
		t.Run(fmt.Sprint(fill), func(t *testing.T) {
			p := &RBACPermissionElem{Key: "root", Children: []*RBACPermissionElem{{Key: "sys", Children: []*RBACPermissionElem{{Key: "read"}}}}}
			if fill {
				require.NoError(t, p.FillDefault(""))
			}
			require.True(t, p.HasPerm2("root.sys.read.child"), "legacy read grant lost")
			p.Cut("root.sys.read")
			rbacAssertPermissions(t, p, rbacTestProbes, func(q string) bool { return q == "" })
		})
	}
	legacyBroad := &RBACPermissionElem{Key: "root"}
	require.True(t, legacyBroad.HasPerm2("root.admin"), "legacy broad grant lost")
	legacyBroad.Intersection(rbacExplicitTree("root.sys.read"))
	rbacAssertPermissions(t, legacyBroad, rbacTestProbes, func(q string) bool { return rbacOracle([]string{"root.sys.read"}, q) })
	var empty RBACPermissionElem
	require.NoError(t, json.Unmarshal([]byte(`{"key":"root","grant":"none"}`), &empty))
	require.False(t, empty.HasPerm2("root"), "explicit non-grant ignored")
}

// TestRBACDecodeReplacesExistingState checks successful replacement and
// atomic failure when decoding into an already populated receiver.
func TestRBACDecodeReplacesExistingState(t *testing.T) {
	for _, scan := range []bool{false, true} {
		t.Run(fmt.Sprint(scan), func(t *testing.T) {
			p := rbacExplicitTree("root")
			data := []byte(`{"key":"root","children":[{"key":"sys","children":[{"key":"read"}]}]}`)
			var err error
			if scan {
				err = p.Scan(data)
			} else {
				err = json.Unmarshal(data, p)
			}
			require.NoError(t, err)
			rbacAssertPermissions(t, p, rbacTestProbes, func(q string) bool { return rbacOracle([]string{"root.sys.read"}, q) })
			before, err := json.Marshal(p)
			require.NoError(t, err)
			require.Error(t, json.Unmarshal([]byte(`{"key":123}`), p))
			after, err := json.Marshal(p)
			require.NoError(t, err)
			require.Equal(t, before, after, "failed decode mutated live state")
		})
	}
}

// TestRBACIdentityNilAndMetadata checks nil policies, disjoint identities,
// and display-title changes that must not change authorization identities.
func TestRBACIdentityNilAndMetadata(t *testing.T) {
	for _, overwrite := range []bool{false, true} {
		p := rbacExplicitTree("root.sys.read")
		q := &RBACPermissionElem{Key: "root", FullKey: "other", Grant: RBACGrantSubtree}
		if overwrite {
			p.OverwriteBy(q, true)
		} else {
			p.Intersection(q)
		}
		rbacAssertPermissions(t, p, rbacTestProbes, func(q string) bool { return q == "" })
		p = rbacExplicitTree("root.sys.read")
		if overwrite {
			p.OverwriteBy(nil, true)
		} else {
			p.Intersection(nil)
		}
		rbacAssertPermissions(t, p, rbacTestProbes, func(q string) bool { return q == "" })
	}
	p := rbacExplicitTree("root.sys.read")
	q := rbacExplicitTree("root.sys.read")
	q.GetElemByKey("root.sys.read").Title = "Updated Read"
	p.OverwriteBy(q, true)
	require.Equal(t, "Updated Read", p.GetElemByKey("root.sys.read").Title, "title not overwritten")
	q.GetElemByKey("root.sys.read").FullKey = "root"
	p.OverwriteBy(q, false)
	require.False(t, p.HasPerm2("root.sys.admin"), "display overwrite changed authority identity")
	var nilTree *RBACPermissionElem
	nilTree.Intersection(p)
	nilTree.OverwriteBy(p, true)
	nilTree.Cut("root")
}

// TestRBACSelfAndReciprocalIntersections checks self and concurrent
// opposite-direction operations for termination without deadlocks.
func TestRBACSelfAndReciprocalIntersections(t *testing.T) {
	done := make(chan struct{})
	go func() {
		defer close(done)
		p := rbacExplicitTree("root.sys.read")
		p.Intersection(p)
		p.OverwriteBy(p, true)
		var wg sync.WaitGroup
		q := rbacExplicitTree("root.sys.write")
		for i := 0; i < 20; i++ {
			wg.Add(2)
			go func() { defer wg.Done(); p.Intersection(q) }()
			go func() { defer wg.Done(); q.OverwriteBy(p, true) }()
		}
		wg.Wait()
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		require.FailNow(t, "intersection deadlocked")
	}
}

// TestRBACMixedOperationSequence checks that combining mutations never
// resurrects a grant already revoked from a structural node.
func TestRBACMixedOperationSequence(t *testing.T) {
	p := rbacExplicitTree("root.sys.read")
	p.Cut("root.sys.read")
	p.UnionAndOverwriteBy(rbacExplicitTree("root.data.audit"))
	p.Intersection(rbacExplicitTree("root.data.*"))
	p.OverwriteBy(rbacExplicitTree("root.data.audit"), true)
	rbacAssertPermissions(t, p, rbacTestProbes, func(q string) bool { return rbacOracle([]string{"root.data.audit"}, q) })
}

// FuzzRBACRestrictionMonotonicity generates policy sets and namespace
// names to check intersection, persistence, and subsequent revocation.
func FuzzRBACRestrictionMonotonicity(f *testing.F) {
	for _, pair := range [][2]uint64{{0, 0}, {1, 8}, {8, 16}, {4, 8}, {63, 17}, {2, 4}} {
		f.Add(pair[0], pair[1], uint8(0))
	}
	f.Fuzz(func(t *testing.T, a, b uint64, cut uint8) {
		left, right := rbacMaskGrants(a), rbacMaskGrants(b)
		namespace := fmt.Sprintf("ns%x", (a>>6)^(b>>6))
		rename := func(keys []string) []string {
			out := make([]string, len(keys))
			for i, key := range keys {
				out[i] = strings.ReplaceAll(key, "sys", namespace)
			}
			return out
		}
		left, right = rename(left), rename(right)
		probes, targets := rename(rbacTestProbes), rename(rbacTestGrantKeys)
		for _, overwrite := range []bool{false, true} {
			p := rbacExplicitTree(left...)
			q := rbacExplicitTree(right...)
			if overwrite {
				p.OverwriteBy(q, true)
			} else {
				p.Intersection(q)
			}
			rbacAssertPermissions(t, p, probes, func(q string) bool { return rbacOracle(left, q) && rbacOracle(right, q) })
			wire, err := json.Marshal(p)
			require.NoError(t, err)
			var restored RBACPermissionElem
			require.NoError(t, json.Unmarshal(wire, &restored))
			rbacAssertPermissions(t, &restored, probes, func(q string) bool { return rbacOracle(left, q) && rbacOracle(right, q) })
			target := targets[int(cut)%len(targets)]
			restored.Cut(RBACPermFullKey(target))
			for _, probe := range probes {
				require.Falsef(t, restored.HasPerm2(RBACPermFullKey(probe)) && (!rbacOracle(left, probe) || !rbacOracle(right, probe)), "sequence gained %q", probe)
			}
		}
	})
}

// TestRBACWildcardAndSubtreeBoundaries checks wildcard, Unicode,
// segment-boundary, and detached-subtree intersection semantics.
func TestRBACWildcardAndSubtreeBoundaries(t *testing.T) {
	pairs := [][2][]string{
		{{"root.*"}, {"root"}},
		{{"root.sys.*"}, {"root.*"}},
		{{"root.sys.*"}, {"root.sys"}},
		{{"root.sys.read.*"}, {"root.sys.read"}},
		{{"root.sys.*"}, {"root.sysadmin"}},
		{{"root.sys.读取"}, {"root.sys.*"}},
	}
	probes := append(append([]string{}, rbacTestProbes...), "root.sys.读取", "root.sys.读取.child", "root.sys.read.*")
	for _, pair := range pairs {
		for _, reverse := range []bool{false, true} {
			for _, overwrite := range []bool{false, true} {
				a, b := pair[0], pair[1]
				if reverse {
					a, b = b, a
				}
				p, q := rbacExplicitTree(a...), rbacExplicitTree(b...)
				if overwrite {
					p.OverwriteBy(q, true)
				} else {
					p.Intersection(q)
				}
				rbacAssertPermissions(t, p, probes, func(q string) bool { return rbacOracle(a, q) && rbacOracle(b, q) })
			}
		}
	}
	subtree := &RBACPermissionElem{Key: "sys", FullKey: "root.sys", Grant: RBACGrantSubtree}
	subtree.Intersection(rbacExplicitTree("root.sys.read"))
	rbacAssertPermissions(t, subtree, probes, func(q string) bool { return rbacOracle([]string{"root.sys.read"}, q) })
}

// TestRBACInvalidModeFailsClosed checks that validation and revocation
// never turn an unrecognized grant mode into an effective grant.
func TestRBACInvalidModeFailsClosed(t *testing.T) {
	p := rbacExplicitTree("root.sys.read")
	p.Grant = RBACGrantMode("unrecognized")
	require.Error(t, p.Valid(), "invalid mode accepted")
	require.False(t, p.HasPerm2("root.sys.read"), "invalid mode authorized")
	p.Cut("root.data.audit")
	require.False(t, p.HasPerm2("root.sys.read"), "cut reactivated invalid state")
}

// TestRBACDepthBoundaryDoesNotSynthesizeGrants checks that truncating
// deep disjoint branches cannot synthesize a new ancestor grant.
func TestRBACDepthBoundaryDoesNotSynthesizeGrants(t *testing.T) {
	makeDeep := func(leaf string) *RBACPermissionElem {
		p := &RBACPermissionElem{Key: "root"}
		n := p
		for i := 0; i < rbacMaxDepth; i++ {
			c := &RBACPermissionElem{Key: "x"}
			n.Children = []*RBACPermissionElem{c}
			n = c
		}
		n.Children = []*RBACPermissionElem{{Key: RBACPermKey(leaf)}}
		return p
	}
	for _, overwrite := range []bool{false, true} {
		p := makeDeep("read")
		q := makeDeep("write")
		if overwrite {
			p.OverwriteBy(q, true)
		} else {
			p.Intersection(q)
		}
		key := "root"
		for i := 0; i < rbacMaxDepth; i++ {
			key += ".x"
			require.Falsef(t, p.HasPerm2(RBACPermFullKey(key+".admin")), "depth %d became a grant", i)
		}
	}
}
