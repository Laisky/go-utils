package utils

import (
	"database/sql/driver"
	"strings"

	"github.com/Laisky/errors/v2"
	"github.com/Laisky/zap"

	"github.com/Laisky/go-utils/v6/json"
	"github.com/Laisky/go-utils/v6/log"
)

const (
	// rbacPermKeyDelimiter delimiter for full key
	rbacPermKeyDelimiter = "."
	// rbacPermWildcardSuffix is the wildcard suffix for all descendant nodes.
	rbacPermWildcardSuffix = rbacPermKeyDelimiter + "*"

	rbacPermissionElemKeyRoot RBACPermKey = "root"
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
// the whole permission tree can represented by the head node
type RBACPermissionElem struct {
	// Title display name of this element
	Title string `json:"title" binding:"min=1"`
	// Key element's identity
	Key RBACPermKey `json:"key,omitempty"`
	// FullKey within all ancester keys, demilite by rbacPermKeyDelimiter
	FullKey  RBACPermFullKey       `json:"full_key,omitempty"`
	Children []*RBACPermissionElem `json:"children,omitempty"`
}

// NewPermissionTree new permission tree only contains root node
func NewPermissionTree() *RBACPermissionElem {
	return &RBACPermissionElem{
		Title:    "root",
		Key:      rbacPermissionElemKeyRoot,
		Children: []*RBACPermissionElem{},
	}
}

// Clone clone permission tree
func (p *RBACPermissionElem) Clone() *RBACPermissionElem {
	newP := *p

	// clone children
	newP.Children = make([]*RBACPermissionElem, len(p.Children))
	for k, c := range p.Children {
		newP.Children[k] = c.Clone()
	}

	return &newP
}

// FillDefault auto filling some default valus
//
// it is best to call this function immediately after initialization
func (p *RBACPermissionElem) FillDefault(ancesterKey RBACPermFullKey) error {
	if p.Key == "" {
		return errors.Errorf("key is empty")
	}

	p.FullKey = ancesterKey.Append(p.Key)
	if p.Title == "" {
		p.Title = p.Key.String()
	}

	if p.Children == nil {
		p.Children = []*RBACPermissionElem{}
	}

	for i := range p.Children {
		if err := p.Children[i].FillDefault(p.FullKey); err != nil {
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
		if p.Children[i].HasPerm(requiredKey) {
			return true
		}
	}

	return false
}

// HasPerm2 checks whether the tree grants the required key by leaf permission nodes.
//
// Params:
//   - requiredKey: required permission key.
//
// Returns:
//   - true if any leaf permission in this tree grants requiredKey.
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

	return p.hasPerm2WithParent(requiredKey, "")
}

// hasPerm2WithParent checks HasPerm2 recursively and computes FullKey when it is missing.
//
// Params:
//   - requiredKey: required permission key.
//   - parentFullKey: parent full key of current node.
//
// Returns:
//   - true if current subtree grants requiredKey.
func (p *RBACPermissionElem) hasPerm2WithParent(requiredKey, parentFullKey RBACPermFullKey) bool {
	if p == nil || p.Key == "" {
		return false
	}

	currentFullKey := p.FullKey
	if currentFullKey == "" {
		currentFullKey = parentFullKey.Append(p.Key)
	}

	if len(p.Children) == 0 {
		return rbacPermissionGrantsRequired(currentFullKey, requiredKey)
	}

	for i := range p.Children {
		if p.Children[i].hasPerm2WithParent(requiredKey, currentFullKey) {
			return true
		}
	}

	return false
}

// Valid valid permission tree
func (p *RBACPermissionElem) Valid() error {
	if p.Key == "" {
		return errors.Errorf("key is empty")
	}

	if p.Children == nil {
		p.Children = []*RBACPermissionElem{}
	}

	for _, v := range p.Children {
		if err := v.Valid(); err != nil {
			return errors.Wrapf(err, "`%s`", v.FullKey.String())
		}
	}

	return nil
}

// UnionAndOverwriteBy merge(union) another tree into this tree by key comparison
func (p *RBACPermissionElem) UnionAndOverwriteBy(other *RBACPermissionElem) {
	if p.Key == "" || other.Key == "" || p.Key != other.Key {
		return
	}

	// replace element's content by another tree
	c := p.Children
	*p = *other
	p.Children = c

	var replacedEle *RBACPermissionElem
	for _, oe := range other.Children {
		for i := range p.Children {
			if p.Children[i].Key == oe.Key {
				replacedEle = p.Children[i]
			}
		}

		// do not has same key element, create new
		if replacedEle == nil {
			p.Children = append(p.Children, oe)
			continue
		}

		// replace element
		replacedEle.UnionAndOverwriteBy(oe)
		replacedEle = nil
	}
}

// Intersection intersect with other permission tree
func (p *RBACPermissionElem) Intersection(other *RBACPermissionElem) {
	if p.Key == "" || other.Key == "" || p.Key != other.Key {
		return
	}

	var filteredChildren []*RBACPermissionElem
	for i := range p.Children {
		for j := range other.Children {
			if other.Children[j].Key == p.Children[i].Key {
				// found, recur
				p.Children[i].Intersection(other.Children[j])
				filteredChildren = append(filteredChildren, p.Children[i])
				break
			}
		}
	}

	p.Children = filteredChildren
}

// OverwriteBy overwrite element's content by another tree,
// but do not append any element from another tree if not exists in current tree.
//
// Args:
//   - intersection: if set to true, will intersect by another tree
func (p *RBACPermissionElem) OverwriteBy(another *RBACPermissionElem, intersection bool) {
	if p.Key == "" || another.Key == "" || p.Key != another.Key {
		return
	}

	c := p.Children
	*p = *another
	p.Children = c

	var filteredChildren []*RBACPermissionElem
	for i := range p.Children {
		for j := range another.Children {
			if another.Children[j].Key == p.Children[i].Key {
				p.Children[i].OverwriteBy(another.Children[j], intersection)
				if intersection {
					filteredChildren = append(filteredChildren, p.Children[i])
				}

				break
			}
		}
	}

	if intersection {
		p.Children = filteredChildren
	}
}

// Cut removes the specified node
//
// Args:
//   - key: in the format like `root.sys.a.b`, or `root.sys.a.*`
//
// The root node cannot be removed.
// You can use `*` as a wildcard to represent removing all child nodes.
func (p *RBACPermissionElem) Cut(key RBACPermFullKey) {
	if p.Key == "" || key == "" || p.Key.String() == key.String() {
		return
	}

	var filteredChildren []*RBACPermissionElem
	for i := range p.Children {
		if rbacCutTargetMatchesNode(key, p.Children[i].FullKey) {
			log.Shared.Debug("cut RBAC permission node",
				zap.String("target_key", key.String()),
				zap.String("removed_key", p.Children[i].FullKey.String()))
			continue
		}

		filteredChildren = append(filteredChildren, p.Children[i])
		p.Children[i].Cut(key)
	}

	p.Children = filteredChildren
}

// GetElemByKey gets the permission tree node by key
//
// Args:
//   - key: permission tree path, like `root.sys`
func (p *RBACPermissionElem) GetElemByKey(key RBACPermFullKey) *RBACPermissionElem {
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
		if ele := p.Children[i].GetElemByKey(key); ele != nil {
			return ele
		}
	}

	return nil
}

// Value implement GORM interface
func (p RBACPermissionElem) Value() (driver.Value, error) {
	b, err := json.Marshal(p)
	if err != nil {
		return nil, errors.Wrap(err, "marshal RBACPermissionElem")
	}
	return string(b), nil
}

// Scan implement GORM interface
func (p *RBACPermissionElem) Scan(input any) error {
	return json.Unmarshal(input.([]byte), p) //nolint:forcetypeassert
}
