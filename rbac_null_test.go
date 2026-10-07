package utils

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestRBACRejectsNullChildrenBeforePublish verifies transactional persistence of authorization state.
func TestRBACRejectsNullChildrenBeforePublish(t *testing.T) {
	for _, raw := range []string{
		`{"key":"root","grant":"none","children":[null]}`,
		`{"key":"root","grant":"none","children":[{"key":"child","children":[null]}]}`,
	} {
		for _, mode := range []string{"scan-string", "scan-bytes", "json"} {
			t.Run(mode+raw, func(t *testing.T) {
				p := &RBACPermissionElem{Key: "original", Grant: RBACGrantNone}
				before, err := json.Marshal(p)
				require.NoError(t, err)
				switch mode {
				case "scan-string":
					err = p.Scan(raw)
				case "scan-bytes":
					err = p.Scan([]byte(raw))
				default:
					err = json.Unmarshal([]byte(raw), p)
				}
				require.Error(t, err)
				after, err := json.Marshal(p)
				require.NoError(t, err)
				require.JSONEq(t, string(before), string(after))
			})
		}
	}
}

// TestRBACManualNullChildrenFailClosed checks defensive traversal of Go-constructed trees.
func TestRBACManualNullChildrenFailClosed(t *testing.T) {
	p := &RBACPermissionElem{Key: "root", FullKey: "root", Grant: RBACGrantNone, Children: []*RBACPermissionElem{nil}}
	require.NotPanics(t, func() { require.Error(t, p.Valid()) })
	require.NotPanics(t, func() { require.Error(t, p.FillDefault("")) })
	require.NotPanics(t, func() { require.False(t, p.HasPerm("root.any")) })
}

// TestRBACInvalidDecodePreservesReceiver covers grants, null roots, and depth boundaries.
func TestRBACInvalidDecodePreservesReceiver(t *testing.T) {
	nested := func(depth int) string {
		return strings.Repeat(`{"key":"branch","children":[`, depth) + `{"key":"leaf"}` + strings.Repeat(`]}`, depth)
	}
	p := &RBACPermissionElem{Key: "original", Grant: RBACGrantNone}
	for _, raw := range []string{
		`null`, `{"key":"root","grant":"unsupported"}`,
		`{"key":"root","children":[{"key":"child","grant":"unsupported"}]}`,
		nested(rbacMaxDepth + 1),
	} {
		before, err := json.Marshal(p)
		require.NoError(t, err)
		require.Error(t, p.Scan(raw))
		after, err := json.Marshal(p)
		require.NoError(t, err)
		require.JSONEq(t, string(before), string(after))
	}
	require.NoError(t, p.Scan(nested(rbacMaxDepth)))
	require.NoError(t, p.Valid())
	require.NoError(t, p.FillDefault(""))
	require.NoError(t, p.Scan(`{"key":"root","grant":"none"}`))
	require.Empty(t, p.Children)
	require.False(t, p.HasPerm2("root.any"))
	require.NoError(t, p.Scan(`{"key":"root"}`))
	require.True(t, p.HasPerm2("root.any"))
}

// TestRBACNilReceiverTraversalReturnsErrors verifies defensive exported helpers.
func TestRBACNilReceiverTraversalReturnsErrors(t *testing.T) {
	var p *RBACPermissionElem
	require.Error(t, p.Valid())
	require.Error(t, p.FillDefault(""))
	require.Error(t, p.Scan(`{"key":"root"}`))
	require.False(t, p.HasPerm("root"))
	require.False(t, p.HasPerm2("root"))
	require.True(t, p.HasPerm(""))
}

// FuzzRBACDecodeNeverPublishesInvalidStructure preserves the receiver on every rejected input.
func FuzzRBACDecodeNeverPublishesInvalidStructure(f *testing.F) {
	for _, raw := range []string{`{"key":"root"}`, `{"key":"root","children":[null]}`, `null`, `{}`} {
		f.Add(raw)
	}
	f.Fuzz(func(t *testing.T, raw string) {
		if len(raw) > 16*1024 {
			t.Skip()
		}
		p := &RBACPermissionElem{Key: "original", Grant: RBACGrantNone}
		before, err := json.Marshal(p)
		require.NoError(t, err)
		if err := p.Scan(raw); err != nil {
			after, marshalErr := json.Marshal(p)
			require.NoError(t, marshalErr)
			require.JSONEq(t, string(before), string(after))
			return
		}
		require.NotPanics(t, func() { _ = p.Valid(); _ = p.FillDefault(""); _ = p.HasPerm("root.any"); _ = p.HasPerm2("root.any") })
	})
}
