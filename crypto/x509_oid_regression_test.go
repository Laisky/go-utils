package crypto

import (
	"encoding/asn1"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestOIDContains_FailingOptionFailsClosed verifies that OIDContains no longer
// discards the error of a failing option. OIDContains cannot return the error, so
// it fails closed: it logs the error and reports no match, even when the OID is
// present, instead of matching under a partially applied configuration. The
// unexported applyfs propagates the wrapped error. Regression for the
// documentation-pass finding that oidContainsOption.applyfs discarded its error.
func TestOIDContains_FailingOptionFailsClosed(t *testing.T) {
	t.Parallel()

	oids := []asn1.ObjectIdentifier{{1, 2, 3, 4}}
	failing := func(*oidContainsOption) error { return errOptionFailedForTest }

	require.True(t, OIDContains(oids, asn1.ObjectIdentifier{1, 2, 3, 4}))
	require.False(t, OIDContains(oids, asn1.ObjectIdentifier{1, 2, 3, 4}, failing))
	require.False(t, OIDContains(oids, asn1.ObjectIdentifier{1, 2, 3}, MatchPrefix(), failing))
	require.False(t, OIDContains(oids, asn1.ObjectIdentifier{1, 2, 3}, failing, MatchPrefix()))

	opt, err := new(oidContainsOption).applyfs(failing)
	require.Nil(t, opt)
	requireWrappedErr(t, err, errOptionFailedForTest)

	opt, err = new(oidContainsOption).applyfs(MatchPrefix())
	require.NoError(t, err)
	require.True(t, opt.prefix)
}

// TestOIDContains_PrefixMatchesWholeArcs verifies that MatchPrefix compares OIDs
// arc by arc: 1.2.3 is a prefix of 1.2.3.4 but not of 1.2.30 or 1.2.34, and an
// empty OID is not a prefix of everything. The previous string comparison treated
// "1.2.3" as a prefix of "1.2.30", so an unrelated policy could satisfy a check.
func TestOIDContains_PrefixMatchesWholeArcs(t *testing.T) {
	t.Parallel()

	prefix := asn1.ObjectIdentifier{1, 2, 3}
	for _, tc := range []struct {
		name string
		oids []asn1.ObjectIdentifier
		want bool
	}{
		{name: "descendant", oids: []asn1.ObjectIdentifier{{1, 2, 3, 4}}, want: true},
		{name: "deep descendant", oids: []asn1.ObjectIdentifier{{1, 2, 3, 4, 5}}, want: true},
		{name: "exact", oids: []asn1.ObjectIdentifier{{1, 2, 3}}, want: true},
		{name: "sibling with longer last arc", oids: []asn1.ObjectIdentifier{{1, 2, 30}}, want: false},
		{name: "sibling descendant", oids: []asn1.ObjectIdentifier{{1, 2, 34, 1}}, want: false},
		{name: "ancestor", oids: []asn1.ObjectIdentifier{{1, 2}}, want: false},
		{name: "unrelated", oids: []asn1.ObjectIdentifier{{2, 5, 29}}, want: false},
		{name: "empty list", oids: nil, want: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.want, OIDContains(tc.oids, prefix, MatchPrefix()))
		})
	}

	t.Run("empty prefix matches nothing", func(t *testing.T) {
		t.Parallel()
		oids := []asn1.ObjectIdentifier{{1, 2, 3}}
		require.False(t, OIDContains(oids, asn1.ObjectIdentifier{}, MatchPrefix()))
		require.False(t, OIDContains(oids, nil, MatchPrefix()))
	})

	t.Run("without MatchPrefix only exact OIDs match", func(t *testing.T) {
		t.Parallel()
		require.False(t, OIDContains([]asn1.ObjectIdentifier{{1, 2, 3, 4}}, prefix))
	})
}
