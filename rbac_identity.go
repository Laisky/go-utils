package utils

import (
	"strings"

	"github.com/Laisky/errors/v2"
)

// canonicalRBACIdentity resolves one node under parent at depth. A standalone
// root may retain an anchored full key ending in its own key; descendants must
// match their structural parent exactly. Invalid identities return false.
func (p *RBACPermissionElem) canonicalRBACIdentity(parent RBACPermFullKey, depth int) (RBACPermFullKey, bool) {
	if p == nil || depth > rbacMaxDepth || p.Key == "" || strings.Contains(p.Key.String(), rbacPermKeyDelimiter) {
		return "", false
	}
	expected := parent.Append(p.Key)
	if p.FullKey == "" {
		return expected, true
	}
	full := p.FullKey.String()
	if strings.HasPrefix(full, ".") || strings.HasSuffix(full, ".") || strings.Contains(full, "..") {
		return "", false
	}
	if depth == 0 {
		if p.FullKey == RBACPermFullKey(p.Key) || strings.HasSuffix(full, "."+p.Key.String()) {
			return p.FullKey, true
		}
		return "", false
	}
	return expected, p.FullKey == expected
}

// validateRBACIdentities checks a complete tree against its parent path without
// changing it. allowMissing retains legacy decoding of omitted key fields;
// authorization and Valid still reject those incomplete nodes.
func (p *RBACPermissionElem) validateRBACIdentities(parent RBACPermFullKey, depth int, allowMissing bool) error {
	if p == nil || depth > rbacMaxDepth {
		return errors.New("invalid permission tree structure")
	}
	current, valid := p.canonicalRBACIdentity(parent, depth)
	if !valid {
		if !allowMissing || p.Key != "" || p.FullKey != "" {
			return errors.New("permission full key does not match its structural identity")
		}
		current = parent
	}
	for i, child := range p.Children {
		if err := child.validateRBACIdentities(current, depth+1, allowMissing); err != nil {
			return errors.Wrapf(err, "validate child %d identity", i)
		}
	}
	return nil
}

// cloneCanonicalRBACBranch copies only valid identities under parent. Legacy
// grant modes are materialized before filtering, so removing denied descendants
// cannot convert their structural parent into a leaf grant. Invalid nodes and
// their descendants are omitted without changing the source tree.
func (p *RBACPermissionElem) cloneCanonicalRBACBranch(parent RBACPermFullKey, depth int) *RBACPermissionElem {
	current, valid := p.canonicalRBACIdentity(parent, depth)
	if !valid || !p.validGrantMode() {
		return nil
	}
	clone := &RBACPermissionElem{
		Key: p.Key, FullKey: current, Title: p.Title, Grant: p.explicitGrantMode(),
	}
	for _, child := range p.Children {
		if copied := child.cloneCanonicalRBACBranch(current, depth+1); copied != nil {
			clone.Children = append(clone.Children, copied)
		}
	}
	return clone
}
