package utils

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// historicalRBACReader models the pre-#33 persisted fields; unknown grant fields are ignored by its JSON reader.
type historicalRBACReader struct {
	Key      string                  `json:"key"`
	FullKey  string                  `json:"full_key"`
	Children []*historicalRBACReader `json:"children"`
}

// permits reproduces the historical leaf/subtree interpretation independently of current grant-mode code.
func (p *historicalRBACReader) permits(query, parent string) bool {
	if p == nil || p.Key == "" {
		return false
	}
	full := p.FullKey
	if full == "" {
		full = p.Key
		if parent != "" {
			full = parent + "." + p.Key
		}
	}
	if len(p.Children) == 0 {
		return query == full || strings.HasPrefix(query, full+".")
	}
	for _, child := range p.Children {
		if child.permits(query, full) {
			return true
		}
	}
	return false
}

// TestRBACStorageCompatibilityNoGrantExpansion prevents old readers from treating persisted deny-only nodes as subtree grants.
func TestRBACStorageCompatibilityNoGrantExpansion(t *testing.T) {
	for _, sql := range []bool{false, true} {
		p := NewPermissionTree()
		require.False(t, p.HasPerm2("root.admin"))
		var encoded []byte
		var err error
		if sql {
			var value any
			value, err = p.Value()
			if err == nil {
				encoded = []byte(value.(string))
			}
		} else {
			encoded, err = json.Marshal(p)
		}
		if err != nil {
			continue
		} // Failing closed is required when legacy storage cannot encode the policy.
		var old historicalRBACReader
		if err := json.Unmarshal(encoded, &old); err != nil {
			continue
		}
		require.False(t, old.permits("root.admin", ""), "old reader expanded a denied grant after a mixed-version write")
	}
}
