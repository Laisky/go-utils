package utils

import (
	"github.com/Laisky/zap"

	"github.com/Laisky/go-utils/v6/log"
)

// Cut removes a node or wildcard descendants and revokes overlapping grants.
// The receiver remains as a structural root even when its grants are revoked.
// A bare "*" removes the receiver's descendants. Missing FullKey values are
// resolved exactly as in HasPerm2.
//
// This positive-grant model cannot represent exclusions from a broad grant.
// Cutting inside such a grant conservatively revokes that entire grant; it may
// deny unrelated descendants rather than leave the requested permission active.
func (p *RBACPermissionElem) Cut(key RBACPermFullKey) {
	if p == nil || key == "" {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if key == "*" {
		key = p.rbacFullKey("").Append("*")
	}
	p.cutWithParent(key, "", 0)
}

// cutWithParent revokes grants overlapping key within the locked receiver.
// Parent resolves missing full keys and depth bounds recursion. Empty or invalid
// branches remain explicitly non-granting; the receiver object is retained.
func (p *RBACPermissionElem) cutWithParent(key, parent RBACPermFullKey, depth int) {
	if p == nil {
		return
	}
	current := p.rbacFullKey(parent)
	// Freeze BEFORE any filtering, including depth-limit truncation.
	invalid := !p.validGrantMode()
	p.Grant = p.explicitGrantMode()
	if depth > rbacMaxDepth || p.Key == "" || invalid {
		p.Grant = RBACGrantNone
		p.Children = nil
		return
	}
	if p.Grant == RBACGrantSubtree &&
		(rbacPermissionGrantsRequired(current, key) ||
			rbacPermissionGrantsRequired(key, current)) {
		p.Grant = RBACGrantNone
	}
	if current == key {
		p.Children = nil
		return
	}
	var filteredChildren []*RBACPermissionElem
	for _, child := range p.Children {
		if child == nil {
			continue
		}
		childKey := child.rbacFullKey(current)
		if rbacCutTargetMatchesNode(key, childKey) {
			log.Shared.Debug("cut RBAC permission node",
				zap.String("target_key", key.String()),
				zap.String("removed_key", childKey.String()))
			continue
		}
		child.cutWithParent(key, current, depth+1)
		filteredChildren = append(filteredChildren, child)
	}
	p.Children = filteredChildren
}
