package utils

import (
	"strings"
	"sync"

	"github.com/Laisky/errors/v2"
)

const (
	// rbacPermKeyDelimiter delimiter for full key
	rbacPermKeyDelimiter = "."
	// rbacPermWildcardSuffix is the wildcard suffix for all descendant nodes.
	rbacPermWildcardSuffix = rbacPermKeyDelimiter + "*"

	rbacPermissionElemKeyRoot RBACPermKey = "root"

	// rbacMaxDepth is the maximum allowed tree depth to prevent stack overflow
	// from maliciously or accidentally deep trees.
	rbacMaxDepth = 128
)

// RBACPermKey permission identity keyword
//
// format like `a`
type RBACPermKey string

// String to string
func (p RBACPermKey) String() string {
	return string(p)
}

// RBACPermFullKey key with ancesters
type RBACPermFullKey string

// String to string
func (p RBACPermFullKey) String() string {
	return string(p)
}

// Parent get element parent key
func (p RBACPermFullKey) Parent() RBACPermFullKey {
	if p == "" {
		return p
	}

	ks := strings.Split(p.String(), rbacPermKeyDelimiter)
	ks = ks[:len(ks)-1]
	return RBACPermFullKey(strings.Join(ks, rbacPermKeyDelimiter))
}

// Append new key to full key
func (p RBACPermFullKey) Append(key RBACPermKey) RBACPermFullKey {
	if p == "" {
		return RBACPermFullKey(key)
	}

	return RBACPermFullKey(strings.Join([]string{p.String(), key.String()}, rbacPermKeyDelimiter))
}

// rbacPermissionGrantsRequired checks whether the permission key grants the required key.
//
// Params:
//   - permissionKey: granted permission key.
//   - requiredKey: required permission key.
//
// Returns:
//   - true if permissionKey grants requiredKey under current RBAC matching rules.
func rbacPermissionGrantsRequired(permissionKey, requiredKey RBACPermFullKey) bool {
	permission := permissionKey.String()
	required := requiredKey.String()
	if required == "" {
		return true
	}

	if permission == "" {
		return false
	}

	if permission == required {
		return true
	}

	if strings.HasSuffix(permission, rbacPermWildcardSuffix) {
		// `root.sys.*` matches descendants only.
		parentPermission := strings.TrimSuffix(permission, rbacPermWildcardSuffix)
		if required == parentPermission {
			return false
		}

		return hasRBACHierarchicalPrefix(required, parentPermission)
	}

	return hasRBACHierarchicalPrefix(required, permission)
}

// rbacCutTargetMatchesNode checks whether a tree node should be removed by Cut target key.
//
// Params:
//   - targetKey: Cut target key, supports exact key and wildcard suffix `.*`.
//   - nodeKey: current tree node key.
//
// Returns:
//   - true if the node should be removed.
func rbacCutTargetMatchesNode(targetKey, nodeKey RBACPermFullKey) bool {
	target := targetKey.String()
	node := nodeKey.String()
	if target == "" || node == "" {
		return false
	}

	if target == node {
		return true
	}

	if strings.HasSuffix(target, rbacPermWildcardSuffix) {
		parentTarget := strings.TrimSuffix(target, rbacPermWildcardSuffix)
		if node == parentTarget {
			return false
		}

		return hasRBACHierarchicalPrefix(node, parentTarget)
	}

	return false
}

// hasRBACHierarchicalPrefix checks whether childKey is a direct descendant path of parentKey.
//
// Params:
//   - childKey: full key of child path.
//   - parentKey: full key of parent path.
//
// Returns:
//   - true if childKey is under parentKey in RBAC hierarchy with a delimiter boundary.
func hasRBACHierarchicalPrefix(childKey, parentKey string) bool {
	if parentKey == "" {
		return true
	}

	if !strings.HasPrefix(childKey, parentKey) {
		return false
	}

	if len(childKey) <= len(parentKey) {
		return false
	}

	return childKey[len(parentKey)] == rbacPermKeyDelimiter[0]
}

// RBACPermissionElem element node of permission tree
//
// the whole permission tree can represented by the head node.
//
// Calls through the same tree root are synchronized. Direct field access and
// operations through child pointers require external synchronization; a child
// mutex does not synchronize access through its parent. Use Value for a locked
// serialization snapshot rather than marshaling a concurrently mutated tree.
type RBACPermissionElem struct {
	mu sync.RWMutex `json:"-"`
	// Title display name of this element
	Title string `json:"title" binding:"min=1"`
	// Key element's identity
	Key RBACPermKey `json:"key,omitempty"`
	// FullKey within all ancester keys, demilite by rbacPermKeyDelimiter
	FullKey  RBACPermFullKey       `json:"full_key,omitempty"`
	Children []*RBACPermissionElem `json:"children,omitempty"`
	// Grant separates authorization from tree shape. The zero value accepts
	// legacy leaf grants; mutation methods materialize that legacy state before
	// changing children. Explicit RBACGrantNone survives JSON/SQL round trips.
	Grant RBACGrantMode `json:"grant,omitempty"`
}

// NewPermissionTree creates a structural root with no grants.
func NewPermissionTree() *RBACPermissionElem {
	return &RBACPermissionElem{
		Title:    "root",
		Key:      rbacPermissionElemKeyRoot,
		Grant:    RBACGrantNone,
		Children: []*RBACPermissionElem{},
	}
}

// Clone returns a detached, depth-bounded copy under the root read lock.
// A nil receiver returns nil; over-depth branches remain non-granting.
func (p *RBACPermissionElem) Clone() *RBACPermissionElem {
	if p == nil {
		return nil
	}
	p.mu.RLock()
	defer p.mu.RUnlock()

	return p.cloneUnlocked()
}

// cloneUnlocked is the lock-free inner implementation of Clone.
func (p *RBACPermissionElem) cloneUnlocked() *RBACPermissionElem {
	return p.cloneRBACAtDepth(0)
}

// cloneRBACAtDepth returns a detached copy of p at the supplied tree depth.
// Nil children remain nil; over-depth or cyclic branches become non-granting
// sentinels instead of recursing without a bound. The caller holds p's root lock.
func (p *RBACPermissionElem) cloneRBACAtDepth(depth int) *RBACPermissionElem {
	if p == nil {
		return nil
	}
	if depth > rbacMaxDepth {
		return &RBACPermissionElem{Title: p.Title, Key: p.Key, FullKey: p.FullKey, Grant: RBACGrantNone}
	}
	newP := &RBACPermissionElem{
		Title:   p.Title,
		Key:     p.Key,
		FullKey: p.FullKey,
		Grant:   p.Grant,
	}

	newP.Children = make([]*RBACPermissionElem, len(p.Children))
	for k, c := range p.Children {
		newP.Children[k] = c.cloneRBACAtDepth(depth + 1)
	}

	return newP
}

// valueSnapshot returns a detached copy for database serialization.
//
// Returns:
//   - a clone of the permission tree that does not reuse the live mutex state.
func (p *RBACPermissionElem) valueSnapshot() *RBACPermissionElem {
	if p == nil {
		return nil
	}

	return p.Clone()
}

// FillDefault auto filling some default valus
//
// it is best to call this function immediately after initialization
func (p *RBACPermissionElem) FillDefault(ancesterKey RBACPermFullKey) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	return p.fillDefaultUnlocked(ancesterKey, 0)
}

// fillDefaultUnlocked initializes identities and grant modes under the root lock.
// AncesterKey supplies the parent path; depth bounds recursion. It returns the
// first invalid key, grant mode, or excessive-depth error.
func (p *RBACPermissionElem) fillDefaultUnlocked(ancesterKey RBACPermFullKey, depth int) error {
	if depth > rbacMaxDepth {
		return errors.Errorf("tree depth exceeds maximum %d", rbacMaxDepth)
	}

	if p.Key == "" {
		return errors.Errorf("key is empty")
	}

	if !p.validGrantMode() {
		return errors.Errorf("invalid grant mode %q", p.Grant)
	}
	p.Grant = p.explicitGrantMode()
	p.FullKey = ancesterKey.Append(p.Key)
	if p.Title == "" {
		p.Title = p.Key.String()
	}

	if p.Children == nil {
		p.Children = []*RBACPermissionElem{}
	}

	for i := range p.Children {
		if err := p.Children[i].fillDefaultUnlocked(p.FullKey, depth+1); err != nil {
			return errors.Wrapf(err, "fill default for `%s`", p.Children[i].FullKey.String())
		}
	}

	return nil
}

// HasPerm check whether has specified key
//
//	| user perms   | required key | match  |
//	| :----------: | :----------: | :---:  |
//	|   `"root"`   |   `"root"`   |   ✅   |
//	|     `""`     |   `"root"`   |   ❌   |
//	|   `"root"`   |     `""`     |   ✅   |
//	| `"root.sys"` |   `"root"`   |   ✅   |
//	|   `"root"`   | `"root.sys"` |   ❌   |
//
// Deprecated: HasPerm uses legacy matching semantics where child permission implies parent permission.
// Use HasPerm2 for the newer matching behavior.
func (p *RBACPermissionElem) HasPerm(requiredKey RBACPermFullKey) bool {
	p.mu.RLock()
	defer p.mu.RUnlock()

	return p.hasPermUnlocked(requiredKey, 0)
}

// hasPermUnlocked checks requiredKey with legacy matching at the given depth.
// The caller holds the root read lock; excess depth returns false.
func (p *RBACPermissionElem) hasPermUnlocked(requiredKey RBACPermFullKey, depth int) bool {
	if depth > rbacMaxDepth {
		return false
	}

	if requiredKey.String() == "" { // do not require any perm
		return true
	}

	if p.FullKey == requiredKey {
		return true
	}

	if len(requiredKey) <= len(p.FullKey) {
		return false
	}

	for i := range p.Children {
		if p.Children[i].hasPermUnlocked(requiredKey, depth+1) {
			return true
		}
	}

	return false
}

// HasPerm2 checks whether an explicit grant in the tree covers the required key.
// Legacy nodes without a Grant mode retain their original leaf semantics until
// initialized or mutated. Structural nodes explicitly marked RBACGrantNone never
// become grants when their children are removed.
//
// Params:
//   - requiredKey: required permission key.
//
// Returns:
//   - true if any effective grant in this tree grants requiredKey.
//
// Matching rules:
//   - An empty required key is always allowed.
//   - An empty granted permission means no permission.
//   - A permission grants itself and all descendants.
//   - A wildcard permission like `root.sys.*` grants descendants only, not `root.sys` itself.
func (p *RBACPermissionElem) HasPerm2(requiredKey RBACPermFullKey) bool {
	if requiredKey == "" {
		return true
	}

	if p == nil {
		return false
	}

	p.mu.RLock()
	defer p.mu.RUnlock()

	return p.hasPerm2WithParent(requiredKey, "", 0)
}

// hasPerm2WithParent checks HasPerm2 recursively and computes FullKey when it is missing.
//
// Params:
//   - requiredKey: required permission key.
//   - parentFullKey: parent full key of current node.
//
// Returns:
//   - true if current subtree grants requiredKey.
func (p *RBACPermissionElem) hasPerm2WithParent(requiredKey, parentFullKey RBACPermFullKey, depth int) bool {
	if depth > rbacMaxDepth {
		return false
	}

	if p == nil || p.Key == "" || !p.validGrantMode() {
		return false
	}

	currentFullKey := p.FullKey
	if currentFullKey == "" {
		currentFullKey = parentFullKey.Append(p.Key)
	}

	if p.explicitGrantMode() == RBACGrantSubtree &&
		rbacPermissionGrantsRequired(currentFullKey, requiredKey) {
		return true
	}

	for i := range p.Children {
		if p.Children[i].hasPerm2WithParent(requiredKey, currentFullKey, depth+1) {
			return true
		}
	}

	return false
}

// Valid validates the permission tree without modifying it.
func (p *RBACPermissionElem) Valid() error {
	p.mu.RLock()
	defer p.mu.RUnlock()

	return p.validUnlocked(0)
}

// validUnlocked checks keys and grant modes at the supplied depth without
// mutating the tree. It returns the first validation or depth-limit error.
func (p *RBACPermissionElem) validUnlocked(depth int) error {
	if depth > rbacMaxDepth {
		return errors.Errorf("tree depth exceeds maximum %d", rbacMaxDepth)
	}

	if p.Key == "" {
		return errors.Errorf("key is empty")
	}

	if !p.validGrantMode() {
		return errors.Errorf("invalid grant mode %q", p.Grant)
	}

	for _, v := range p.Children {
		if err := v.validUnlocked(depth + 1); err != nil {
			return errors.Wrapf(err, "`%s`", v.FullKey.String())
		}
	}

	return nil
}

// UnionAndOverwriteBy merges grants and titles from other into the receiver.
// Nodes must have matching keys and resolved authorization identities. Invalid
// grant modes contribute no grants. The source is snapshotted without holding
// the receiver lock, so reciprocal calls cannot deadlock. Other is not modified.
func (p *RBACPermissionElem) UnionAndOverwriteBy(other *RBACPermissionElem) {
	if p == nil || other == nil || p == other {
		return
	}
	otherSnapshot := other.Clone()
	p.mu.Lock()
	defer p.mu.Unlock()

	p.unionAndOverwriteByUnlocked(otherSnapshot, "", 0)
}

// unionAndOverwriteByUnlocked merges a detached source into a locked receiver.
// Parent and depth identify the current location; neither display metadata nor
// normalization of an invalid mode may turn a previously denied branch into a
// grant. Identity mismatches and invalid source nodes are ignored fail-closed.
func (p *RBACPermissionElem) unionAndOverwriteByUnlocked(other *RBACPermissionElem, parent RBACPermFullKey, depth int) {
	if p == nil || other == nil || depth > rbacMaxDepth {
		return
	}
	if p.Key == "" || other.Key == "" || p.Key != other.Key || !other.validGrantMode() {
		return
	}
	current := p.rbacFullKey(parent)
	if current != other.rbacFullKey(parent) {
		return
	}
	if !p.validGrantMode() {
		// HasPerm2 denies the entire invalid subtree, not just its root.
		p.Grant = RBACGrantNone
		p.Children = nil
	}
	grant := p.explicitGrantMode() == RBACGrantSubtree ||
		other.explicitGrantMode() == RBACGrantSubtree
	p.Grant = RBACGrantNone
	if grant {
		p.Grant = RBACGrantSubtree
	}
	p.Title = other.Title
	p.FullKey = current

	for _, oe := range other.Children {
		if oe == nil || oe.Key == "" || !oe.validGrantMode() {
			continue
		}
		var replacedEle *RBACPermissionElem
		for _, child := range p.Children {
			if child != nil && child.Key == oe.Key {
				replacedEle = child
				break
			}
		}
		if replacedEle == nil {
			p.Children = append(p.Children, oe.cloneRBACAtDepth(depth+1))
			continue
		}
		replacedEle.unionAndOverwriteByUnlocked(oe, current, depth+1)
	}
}

// Intersection retains grants covered by both inputs within the receiver hierarchy.
// Noncanonical FullKey values outside that hierarchy are dropped fail-closed.
// Common structural nodes may remain, but are not themselves grants. A nil
// other tree denotes an empty permission set. Other is not modified.
func (p *RBACPermissionElem) Intersection(other *RBACPermissionElem) {
	if p == nil || p == other {
		return
	}
	// Snapshot the other input before locking the receiver, avoiding both
	// self-deadlock and reversed two-tree lock ordering.
	otherState := snapshotRBACPermissions(other)
	p.mu.Lock()
	defer p.mu.Unlock()
	p.intersectRBACSnapshot(otherState, false)
}

// OverwriteBy updates titles of matching nodes without changing their grants.
// With intersection=true, it also restricts permissions to the semantic
// intersection. This can materialize a narrower branch from another when a
// broad receiver grant covers it; it never grants permissions outside either
// input. FullKey is authorization identity, not overwritable display metadata.
func (p *RBACPermissionElem) OverwriteBy(another *RBACPermissionElem, intersection bool) {
	if p == nil || p == another {
		return
	}
	otherState := snapshotRBACPermissions(another)
	p.mu.Lock()
	defer p.mu.Unlock()
	if intersection {
		p.intersectRBACSnapshot(otherState, true)
		return
	}
	p.overwriteRBACTitles(otherState, "", 0)
}

// GetElemByKey gets the permission tree node by key
//
// Args:
//   - key: permission tree path, like `root.sys`
func (p *RBACPermissionElem) GetElemByKey(key RBACPermFullKey) *RBACPermissionElem {
	p.mu.RLock()
	defer p.mu.RUnlock()

	return p.getElemByKeyUnlocked(key, 0)
}

// getElemByKeyUnlocked returns the node matching key, or nil if absent.
// Depth bounds recursion and the caller holds the tree root read lock.
func (p *RBACPermissionElem) getElemByKeyUnlocked(key RBACPermFullKey, depth int) *RBACPermissionElem {
	if depth > rbacMaxDepth {
		return nil
	}

	if p.Key == "" || key == "" {
		return nil
	}

	if p.FullKey == key {
		return p
	}

	if len(key) <= len(p.FullKey) {
		return nil
	}

	for i := range p.Children {
		if ele := p.Children[i].getElemByKeyUnlocked(key, depth+1); ele != nil {
			return ele
		}
	}

	return nil
}
