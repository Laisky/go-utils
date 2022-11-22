package utils

import (
	"database/sql/driver"
	"strings"

	"github.com/Laisky/errors"
)

const (
	// rbacPermKeyDelimiter delimiter for full key
	rbacPermKeyDelimiter = "."

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

// Contains is contains acquire permission
func (p RBACPermFullKey) Contains(acquire RBACPermFullKey) bool {
	return strings.Index(p.String(), acquire.String()) == 0
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
//	| user prems   | acquired key | match  |
//	| :----------: | :----------: | :---:  |
//	|   `"root"`   |   `"root"`   |   ✅   |
//	|     `""`     |   `"root"`   |   ❌   |
//	|   `"root"`   |     `""`     |   ✅   |
//	| `"root.sys"` |   `"root"`   |   ✅   |
//	|   `"root"`   | `"root.sys"` |   ❌   |
func (p *RBACPermissionElem) HasPerm(acquiredKey RBACPermFullKey) bool {
	if acquiredKey.String() == "" { // do not acquire any perm
		return true
	}

	if p.FullKey == acquiredKey {
		return true
	}

	if len(acquiredKey) <= len(p.FullKey) {
		return false
	}

	for i := range p.Children {
		if p.Children[i].HasPerm(acquiredKey) {
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

// Cut 剪除指定节点
//
// Args:
//
//	key: 形如 `root.sys.a.b`，或 `root.sys.a.*`
//
// 头节点不允许剪除。
// 可以使用 `*` 作为通配符，代表剪除所有子节点。
func (p *RBACPermissionElem) Cut(key RBACPermFullKey) {
	if p.Key == "" || key == "" || p.Key.String() == key.String() {
		return
	}

	var filteredChildren []*RBACPermissionElem
	for i := range p.Children {
		if !key.Contains(p.Children[i].FullKey) {
			filteredChildren = append(filteredChildren, p.Children[i])
			p.Children[i].Cut(key)
		}
	}

	p.Children = filteredChildren
}

// GetElemByKey 通过 key 获取指定的权限树节点
//
// Args:
//   - key: 权限树路径，形如 `root.sys`
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
	return string(b), err
}

// Scan implement GORM interface
func (p *RBACPermissionElem) Scan(input any) error {
	return json.Unmarshal(input.([]byte), p)
}
