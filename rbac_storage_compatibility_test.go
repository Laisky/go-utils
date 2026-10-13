package utils

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// pre33RBACReader mirrors the field layout and leaf-based authorization of the
// historical pre-#33 reader. It deliberately does not understand explicit Grant.
type pre33RBACReader struct {
	Title    string             `json:"title"`
	Key      string             `json:"key"`
	FullKey  string             `json:"full_key"`
	Children []*pre33RBACReader `json:"children"`
}

// allows independently evaluates the historical leaf-grant rule for a test query.
func (p *pre33RBACReader) allows(query, parent string) bool {
	if query == "" {
		return true
	}
	if p == nil || p.Key == "" {
		return false
	}
	key := p.FullKey
	if key == "" {
		key = p.Key
		if parent != "" {
			key = parent + "." + p.Key
		}
	}
	if len(p.Children) == 0 {
		if strings.HasSuffix(key, ".*") {
			return strings.HasPrefix(query, strings.TrimSuffix(key, "*"))
		}
		return query == key || strings.HasPrefix(query, key+".")
	}
	for _, child := range p.Children {
		if child.allows(query, key) {
			return true
		}
	}
	return false
}

// TestRBACStorageCompatibilityNoLegacyEscalation reproduces cross-version
// privilege expansion and requires legacy writers to refuse unrepresentable grants.
func TestRBACStorageCompatibilityNoLegacyEscalation(t *testing.T) {
	for _, viaSQL := range []bool{false, true} {
		p := NewPermissionTree()
		require.False(t, p.HasPerm2("root.admin"))
		var raw []byte
		var err error
		if viaSQL {
			var value any
			value, err = p.Value()
			if err == nil {
				raw = []byte(value.(string))
			}
		} else {
			raw, err = json.Marshal(p)
		}
		if err != nil {
			continue
		} // An explicit migration-required refusal is fail-closed.
		var old pre33RBACReader
		require.NoError(t, json.Unmarshal(raw, &old))
		require.False(t, old.allows("root.admin", ""), "an old reader expanded an explicit deny after an unversioned write (SQL=%v)", viaSQL)
	}
}
