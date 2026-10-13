package utils

import (
	"encoding/json"

	"github.com/Laisky/errors/v2"
)

// snapshotRBACJSONForTest serializes a detached test tree for before/after state
// comparisons, including deliberately invalid fixtures. It is not a persistence
// API and does not permit invalid policies through production storage guards.
func snapshotRBACJSONForTest(p *RBACPermissionElem) ([]byte, error) {
	var convert func(*RBACPermissionElem) *permissionJSON
	convert = func(node *RBACPermissionElem) *permissionJSON {
		if node == nil {
			return nil
		}
		wire := &permissionJSON{Title: node.Title, Key: node.Key, FullKey: node.FullKey, Grant: node.Grant}
		if node.Children != nil {
			wire.Children = make([]*permissionJSON, len(node.Children))
			for i, child := range node.Children {
				wire.Children[i] = convert(child)
			}
		}
		return wire
	}
	data, err := json.Marshal(convert(p.Clone()))
	if err != nil {
		return nil, errors.Wrap(err, "snapshot test RBAC tree")
	}
	return data, nil
}
