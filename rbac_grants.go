package utils

import "strings"

// RBACGrantMode makes the distinction between a structural node and a grant
// persistent. Empty is reserved for backward-compatible legacy input only.
type RBACGrantMode string

const (
	// RBACGrantNone means this node does not grant anything; children may grant.
	RBACGrantNone RBACGrantMode = "none"
	// RBACGrantSubtree grants the node and descendants, or descendants only
	// when its full key ends in ".*".
	RBACGrantSubtree RBACGrantMode = "subtree"
)

func (p *RBACPermissionElem) validGrantMode() bool {
	return p.Grant == "" || p.Grant == RBACGrantNone || p.Grant == RBACGrantSubtree
}

// explicitGrantMode is the legacy-input adapter. Never call it after filtering
// a legacy node's children: materialize the original mode BEFORE mutation.
func (p *RBACPermissionElem) explicitGrantMode() RBACGrantMode {
	if p.Grant == RBACGrantSubtree || (p.Grant == "" && len(p.Children) == 0) {
		return RBACGrantSubtree
	}
	return RBACGrantNone
}

func (p *RBACPermissionElem) rbacFullKey(parent RBACPermFullKey) RBACPermFullKey {
	if p.FullKey != "" {
		return p.FullKey
	}
	return parent.Append(p.Key)
}

type rbacNodeInfo struct {
	title string
}

// rbacPermissionSnapshot is detached from live nodes and mutexes. The grant set
// is canonical authorization data; the node map is display metadata only.
type rbacPermissionSnapshot struct {
	grants map[RBACPermFullKey]struct{}
	nodes  map[RBACPermFullKey]rbacNodeInfo
	// order gives deterministic results without depending on map iteration.
	order []RBACPermFullKey
}

func newRBACSnapshot() rbacPermissionSnapshot {
	return rbacPermissionSnapshot{
		grants: make(map[RBACPermFullKey]struct{}),
		nodes:  make(map[RBACPermFullKey]rbacNodeInfo),
	}
}

func snapshotRBACPermissions(p *RBACPermissionElem) rbacPermissionSnapshot {
	s := newRBACSnapshot()
	if p != nil {
		p.mu.RLock()
		defer p.mu.RUnlock()
		s.collect(p, "", 0)
	}
	return s
}

func (s *rbacPermissionSnapshot) collect(p *RBACPermissionElem, parent RBACPermFullKey, depth int) {
	if p == nil || p.Key == "" || !p.validGrantMode() || depth > rbacMaxDepth {
		return
	}
	key := p.rbacFullKey(parent)
	if _, exists := s.nodes[key]; !exists {
		s.nodes[key] = rbacNodeInfo{title: p.Title}
	}
	if p.explicitGrantMode() == RBACGrantSubtree {
		if _, exists := s.grants[key]; !exists {
			s.grants[key] = struct{}{}
			s.order = append(s.order, key)
		}
	}
	for _, child := range p.Children {
		s.collect(child, key, depth+1)
	}
}

// coversGrant tests containment of an entire positive subtree grant, not just
// overlap. Exact-key and ancestor lookups are O(path depth); wildcard ancestors
// cover descendants but never their own parent. No pairwise quadratic join is
// needed, even for large sibling sets.
func (s rbacPermissionSnapshot) coversGrant(key RBACPermFullKey) bool {
	if key == "" {
		return false
	}
	if _, exists := s.grants[key]; exists {
		return true
	}
	for ancestor := key.Parent(); ancestor != ""; ancestor = ancestor.Parent() {
		if _, exists := s.grants[ancestor]; exists {
			return true
		}
		if _, exists := s.grants[ancestor.Append("*")]; exists {
			return true
		}
	}
	return false
}

func (p *RBACPermissionElem) intersectRBACSnapshot(other rbacPermissionSnapshot, overwrite bool) {
	before := newRBACSnapshot()
	before.collect(p, "", 0)
	// First retain common structure with EVERY retained node explicitly denied.
	// Then install only grants proven to belong to both original grant sets.
	p.retainRBACStructure(other, "", 0, overwrite)
	for _, key := range before.order {
		if other.coversGrant(key) {
			p.insertRBACGrant(key, before, other, overwrite)
		}
	}
	for _, key := range other.order {
		if before.coversGrant(key) {
			p.insertRBACGrant(key, before, other, overwrite)
		}
	}
}

func (p *RBACPermissionElem) retainRBACStructure(other rbacPermissionSnapshot, parent RBACPermFullKey, depth int, overwrite bool) {
	current := p.rbacFullKey(parent)
	invalid := p.Key == "" || !p.validGrantMode() || depth > rbacMaxDepth
	p.Grant = RBACGrantNone
	if invalid {
		p.Children = nil
		return
	}
	if info, exists := other.nodes[current]; overwrite && exists {
		p.Title = info.title
	}
	var children []*RBACPermissionElem
	for _, child := range p.Children {
		if child == nil {
			continue
		}
		if _, exists := other.nodes[child.rbacFullKey(current)]; !exists {
			continue
		}
		child.retainRBACStructure(other, current, depth+1, overwrite)
		children = append(children, child)
	}
	p.Children = children
}

// insertRBACGrant reconstructs a narrower branch when a broad input grant was
// intersected with a descendant grant. Every synthesized ancestor is structural.
func (p *RBACPermissionElem) insertRBACGrant(key RBACPermFullKey, before, other rbacPermissionSnapshot, overwrite bool) {
	root := p.rbacFullKey("")
	if p.Key == "" || root == "" {
		return
	}
	if key == root {
		p.Grant = RBACGrantSubtree
		return
	}
	if !hasRBACHierarchicalPrefix(key.String(), root.String()) {
		// Inconsistent externally supplied FullKey values cannot be safely
		// represented below this root; dropping them is fail-closed.
		return
	}
	segments := strings.Split(strings.TrimPrefix(key.String(), root.String()+rbacPermKeyDelimiter), rbacPermKeyDelimiter)
	if len(segments) > rbacMaxDepth {
		return
	}
	current, node := root, p
	for _, segment := range segments {
		if segment == "" {
			return
		}
		current = current.Append(RBACPermKey(segment))
		var next *RBACPermissionElem
		for _, child := range node.Children {
			if child != nil && child.rbacFullKey(current.Parent()) == current && child.Key == RBACPermKey(segment) {
				next = child
				break
			}
		}
		if next == nil {
			title := segment
			if info, exists := before.nodes[current]; exists {
				title = info.title
			} else if info, exists := other.nodes[current]; exists {
				title = info.title
			}
			if info, exists := other.nodes[current]; overwrite && exists {
				title = info.title
			}
			next = &RBACPermissionElem{Key: RBACPermKey(segment), FullKey: current, Title: title, Grant: RBACGrantNone}
			node.Children = append(node.Children, next)
		}
		node = next
	}
	node.Grant = RBACGrantSubtree
}

func (p *RBACPermissionElem) overwriteRBACTitles(other rbacPermissionSnapshot, parent RBACPermFullKey, depth int) {
	if p == nil || p.Key == "" || depth > rbacMaxDepth {
		return
	}
	current := p.rbacFullKey(parent)
	if info, exists := other.nodes[current]; exists {
		p.Title = info.title
	}
	for _, child := range p.Children {
		child.overwriteRBACTitles(other, current, depth+1)
	}
}
