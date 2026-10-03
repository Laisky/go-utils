package utils

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"
)

// The oracle operates on segment arrays and does not call production matchers.
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

func rbacAssertPermissions(t testing.TB, p *RBACPermissionElem, probes []string, want func(string) bool) {
	t.Helper()
	for _, q := range probes {
		if got := p.HasPerm2(RBACPermFullKey(q)); got != want(q) {
			t.Fatalf("permission %q: got %v, want %v", q, got, want(q))
		}
	}
}

var rbacTestGrantKeys = []string{"root", "root.sys", "root.sys.*", "root.sys.read", "root.sys.write", "root.sysadmin.audit"}
var rbacTestProbes = []string{"", "root", "root.sys", "root.sys.*", "root.sys.read", "root.sys.read.child", "root.sys.write", "root.sys.admin", "root.sys.future.action", "root.sysadmin", "root.sysadmin.audit", "root.sysadmin.audit.child", "root.data", "root.data.audit", "root2", "other.root"}

func rbacMaskGrants(mask uint64) []string {
	var out []string
	for i, key := range rbacTestGrantKeys {
		if mask&(1<<i) != 0 {
			out = append(out, key)
		}
	}
	return out
}

func TestRBACSemanticIntersectionExhaustive(t *testing.T) {
	for a := uint64(0); a < 64; a++ {
		for b := uint64(0); b < 64; b++ {
			left, right := rbacMaskGrants(a), rbacMaskGrants(b)
			for _, overwrite := range []bool{false, true} {
				p, q := rbacExplicitTree(left...), rbacExplicitTree(right...)
				before, _ := json.Marshal(q)
				if overwrite {
					p.OverwriteBy(q, true)
				} else {
					p.Intersection(q)
				}
				for _, probe := range rbacTestProbes {
					want := rbacOracle(left, probe) && rbacOracle(right, probe)
					if got := p.HasPerm2(RBACPermFullKey(probe)); got != want {
						t.Fatalf("a=%d b=%d overwrite=%v query=%q got=%v want=%v", a, b, overwrite, probe, got, want)
					}
				}
				after, _ := json.Marshal(q)
				if string(before) != string(after) {
					t.Fatal("other input was mutated")
				}
			}
		}
	}
}

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
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(b), `"grant":"none"`) {
				t.Fatalf("non-grant state not persisted: %s", b)
			}
			var fromJSON RBACPermissionElem
			if err := json.Unmarshal(b, &fromJSON); err != nil {
				t.Fatal(err)
			}
			value, err := p.Value()
			if err != nil {
				t.Fatal(err)
			}
			var fromSQL RBACPermissionElem
			if err := fromSQL.Scan(value); err != nil {
				t.Fatal(err)
			}
			for name, copy := range map[string]*RBACPermissionElem{"live": p, "clone": p.Clone(), "json": &fromJSON, "sql": &fromSQL} {
				if err := copy.FillDefault(""); err != nil {
					t.Fatal(err)
				}
				rbacAssertPermissions(t, copy, rbacTestProbes, func(q string) bool { return q == "" })
				if sys := copy.GetElemByKey("root.sys"); sys == nil {
					t.Fatalf("%s lost common structural node", name)
				} else if sys.HasPerm2("root.sys.admin") {
					t.Fatalf("%s subtree regranted admin", name)
				}
				// Subsequent metadata operations must not resurrect the emptied parent.
				copy.OverwriteBy(rbacRegressionTree("admin"), false)
				rbacAssertPermissions(t, copy, rbacTestProbes, func(q string) bool { return q == "" })
			}
		})
	}
}

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
				if got && !rbacOracle(grants, probe) {
					t.Fatalf("cut %q grants new %q for %v", target, probe, grants)
				}
				if probe != "" && got && rbacOracle([]string{effectiveTarget}, probe) {
					t.Fatalf("cut %q failed to revoke %q for %v", target, probe, grants)
				}
			}
		}
	}
}

func TestRBACLegacyAndDefaultState(t *testing.T) {
	if NewPermissionTree().HasPerm2("root.admin") {
		t.Fatal("new empty tree grants admin")
	}
	for _, fill := range []bool{false, true} {
		t.Run(fmt.Sprint(fill), func(t *testing.T) {
			p := &RBACPermissionElem{Key: "root", Children: []*RBACPermissionElem{{Key: "sys", Children: []*RBACPermissionElem{{Key: "read"}}}}}
			if fill {
				if err := p.FillDefault(""); err != nil {
					t.Fatal(err)
				}
			}
			if !p.HasPerm2("root.sys.read.child") {
				t.Fatal("legacy read grant lost")
			}
			p.Cut("root.sys.read")
			rbacAssertPermissions(t, p, rbacTestProbes, func(q string) bool { return q == "" })
		})
	}
	legacyBroad := &RBACPermissionElem{Key: "root"}
	if !legacyBroad.HasPerm2("root.admin") {
		t.Fatal("legacy broad grant lost")
	}
	legacyBroad.Intersection(rbacExplicitTree("root.sys.read"))
	rbacAssertPermissions(t, legacyBroad, rbacTestProbes, func(q string) bool { return rbacOracle([]string{"root.sys.read"}, q) })
	var empty RBACPermissionElem
	if err := json.Unmarshal([]byte(`{"key":"root","grant":"none"}`), &empty); err != nil {
		t.Fatal(err)
	}
	if empty.HasPerm2("root") {
		t.Fatal("explicit non-grant ignored")
	}
}

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
			if err != nil {
				t.Fatal(err)
			}
			rbacAssertPermissions(t, p, rbacTestProbes, func(q string) bool { return rbacOracle([]string{"root.sys.read"}, q) })
			before, _ := json.Marshal(p)
			if err := json.Unmarshal([]byte(`{"key":123}`), p); err == nil {
				t.Fatal("expected decode error")
			}
			after, _ := json.Marshal(p)
			if string(before) != string(after) {
				t.Fatal("failed decode mutated live state")
			}
		})
	}
}

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
	if p.GetElemByKey("root.sys.read").Title != "Updated Read" {
		t.Fatal("title not overwritten")
	}
	q.GetElemByKey("root.sys.read").FullKey = "root"
	p.OverwriteBy(q, false)
	if p.HasPerm2("root.sys.admin") {
		t.Fatal("display overwrite changed authority identity")
	}
	var nilTree *RBACPermissionElem
	nilTree.Intersection(p)
	nilTree.OverwriteBy(p, true)
	nilTree.Cut("root")
}

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
		t.Fatal("intersection deadlocked")
	}
}

func TestRBACMixedOperationSequence(t *testing.T) {
	p := rbacExplicitTree("root.sys.read")
	p.Cut("root.sys.read")
	p.UnionAndOverwriteBy(rbacExplicitTree("root.data.audit"))
	p.Intersection(rbacExplicitTree("root.data.*"))
	p.OverwriteBy(rbacExplicitTree("root.data.audit"), true)
	rbacAssertPermissions(t, p, rbacTestProbes, func(q string) bool { return rbacOracle([]string{"root.data.audit"}, q) })
}

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
			if err != nil {
				t.Fatal(err)
			}
			var restored RBACPermissionElem
			if err := json.Unmarshal(wire, &restored); err != nil {
				t.Fatal(err)
			}
			rbacAssertPermissions(t, &restored, probes, func(q string) bool { return rbacOracle(left, q) && rbacOracle(right, q) })
			target := targets[int(cut)%len(targets)]
			restored.Cut(RBACPermFullKey(target))
			for _, probe := range probes {
				if restored.HasPerm2(RBACPermFullKey(probe)) && (!rbacOracle(left, probe) || !rbacOracle(right, probe)) {
					t.Fatalf("sequence gained %q", probe)
				}
			}
		}
	})
}

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

func TestRBACInvalidModeFailsClosed(t *testing.T) {
	p := rbacExplicitTree("root.sys.read")
	p.Grant = RBACGrantMode("unrecognized")
	if p.Valid() == nil {
		t.Fatal("invalid mode accepted")
	}
	if p.HasPerm2("root.sys.read") {
		t.Fatal("invalid mode authorized")
	}
	p.Cut("root.data.audit")
	if p.HasPerm2("root.sys.read") {
		t.Fatal("cut reactivated invalid state")
	}
}

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
			if p.HasPerm2(RBACPermFullKey(key + ".admin")) {
				t.Fatalf("depth %d became a grant", i)
			}
		}
	}
}
