package crypto

import (
	"context"
	"crypto/x509"
	"crypto/x509/pkix"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// validityCaseForTest describes one requested validity window relative to a
// captured clock value.
type validityCaseForTest struct {
	name      string
	notBefore time.Duration
	notAfter  time.Duration
}

// legitimateValidityCasesForTest lists sub-day, exact-day, near-day-boundary
// and multi-day requests that must be issued without extending NotAfter.
func legitimateValidityCasesForTest() []validityCaseForTest {
	return []validityCaseForTest{
		{"plus one hour", 0, time.Hour},
		{"sub-day", 0, 23*time.Hour + 59*time.Minute},
		{"exact day", 0, 24 * time.Hour},
		{"just after day boundary", 0, 24*time.Hour + 2*time.Second},
		{"just before day boundary", 0, 24*time.Hour - 2*time.Second},
		{"multi-day", 0, 72*time.Hour + 17*time.Minute},
		{"backdated not before", -2 * time.Hour, 3 * time.Hour},
		{"future not before", 30 * time.Minute, 90 * time.Minute},
	}
}

// invalidValidityCasesForTest lists past, zero-length and inverted windows
// that must be rejected before issuance.
func invalidValidityCasesForTest() []validityCaseForTest {
	return []validityCaseForTest{
		{"expired one hour ago", -3 * time.Hour, -time.Hour},
		{"expires now", -time.Hour, 0},
		{"zero length", time.Hour, time.Hour},
		{"inverted", 2 * time.Hour, time.Hour},
	}
}

// requireIssuedValidityForTest parses certDer and requires that its validity
// never exceeds the requested window and, apart from one-second truncation,
// matches the requested bounds.
func requireIssuedValidityForTest(t *testing.T, certDer []byte, notBefore, notAfter time.Time) {
	t.Helper()

	cert := parseSMCertForTest(t, certDer)
	require.False(t, cert.NotAfter.After(notAfter),
		"issued NotAfter %s must not exceed requested %s", cert.NotAfter, notAfter)
	require.False(t, cert.NotBefore.Before(notBefore.Truncate(time.Second)),
		"issued NotBefore %s must not precede requested %s", cert.NotBefore, notBefore)
	require.True(t, cert.NotAfter.After(cert.NotBefore))
	// exact validity support records the requested bounds at one-second precision
	require.WithinDuration(t, notAfter, cert.NotAfter, time.Second)
}

// TestTongsuoNewX509CertNeverExtendsNotAfter issues self-signed certificates
// through the real tongsuo binary and verifies the emitted validity never
// extends the requested window, while past, zero and inverted windows are
// rejected before issuance. Regression for issue #40.
func TestTongsuoNewX509CertNeverExtendsNotAfter(t *testing.T) {
	t.Parallel()
	ins := newSecurityTestTongsuo(t)
	ctx := t.Context()

	prikeyPem, err := ins.NewPrikey(ctx)
	require.NoError(t, err)

	issue := func(ctx context.Context, notBefore, notAfter time.Time) ([]byte, error) {
		return ins.NewX509Cert(ctx, prikeyPem,
			WithX509CertCommonName("validity-self-signed"),
			WithX509CertNotBefore(notBefore),
			WithX509CertNotAfter(notAfter),
		)
	}

	for _, tc := range legitimateValidityCasesForTest() {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			now := time.Now().UTC()
			notBefore, notAfter := now.Add(tc.notBefore), now.Add(tc.notAfter)

			certDer, err := issue(ctx, notBefore, notAfter)
			require.NoError(t, err)
			requireIssuedValidityForTest(t, certDer, notBefore, notAfter)
		})
	}

	for _, tc := range invalidValidityCasesForTest() {
		t.Run("reject "+tc.name, func(t *testing.T) {
			t.Parallel()
			now := time.Now().UTC()
			certDer, err := issue(ctx, now.Add(tc.notBefore), now.Add(tc.notAfter))
			require.Error(t, err)
			require.Nil(t, certDer)
		})
	}

	t.Run("reject zero not after", func(t *testing.T) {
		t.Parallel()
		certDer, err := issue(ctx, time.Now().UTC(), time.Time{})
		require.Error(t, err)
		require.Nil(t, certDer)
	})
}

// TestTongsuoNewX509CertByCSRNeverExtendsNotAfter signs real CSRs through the
// tongsuo binary and verifies the emitted validity never extends the requested
// window, while past, zero and inverted windows are rejected before issuance.
// Regression for issue #40.
func TestTongsuoNewX509CertByCSRNeverExtendsNotAfter(t *testing.T) {
	t.Parallel()
	ins := newSecurityTestTongsuo(t)
	ctx := t.Context()

	caKey, caDer := newTongsuoSM2CAForTest(t, ins, "validity-root")
	csrDer := newTongsuoSM2CSRForTest(t, ins, WithX509CSRCommonName("validity-leaf"))

	issue := func(ctx context.Context, notBefore, notAfter time.Time) ([]byte, error) {
		return ins.NewX509CertByCSR(ctx, caDer, caKey, csrDer,
			WithX509SignCSRNotBefore(notBefore),
			WithX509SignCSRNotAfter(notAfter),
		)
	}

	for _, tc := range legitimateValidityCasesForTest() {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			now := time.Now().UTC()
			notBefore, notAfter := now.Add(tc.notBefore), now.Add(tc.notAfter)

			certDer, err := issue(ctx, notBefore, notAfter)
			require.NoError(t, err)
			requireIssuedValidityForTest(t, certDer, notBefore, notAfter)
		})
	}

	for _, tc := range invalidValidityCasesForTest() {
		t.Run("reject "+tc.name, func(t *testing.T) {
			t.Parallel()
			now := time.Now().UTC()
			certDer, err := issue(ctx, now.Add(tc.notBefore), now.Add(tc.notAfter))
			require.Error(t, err)
			require.Nil(t, certDer)
		})
	}
}

// TestTongsuoValidityConversion checks the validity conversion against a fixed
// clock, so day-boundary behavior is deterministic: exact mode encodes the
// truncated bounds, whole-day mode floors with a safety margin and rejects
// anything it cannot represent without extending NotAfter. Regression for
// issue #40.
func TestTongsuoValidityConversion(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 10, 7, 12, 0, 0, 500_000_000, time.UTC)

	t.Run("exact mode keeps truncated bounds", func(t *testing.T) {
		t.Parallel()
		v, err := newTongsuoValidity(now, now.Add(-time.Minute), now.Add(time.Hour+999*time.Millisecond))
		require.NoError(t, err)
		args, err := v.args(now, true)
		require.NoError(t, err)
		require.Equal(t, []string{
			"-not_before", "20261007115900Z",
			"-not_after", "20261007130001Z",
		}, args)
	})

	t.Run("generalized time years", func(t *testing.T) {
		t.Parallel()
		v, err := newTongsuoValidity(now, now, time.Date(2060, 1, 2, 3, 4, 5, 0, time.UTC))
		require.NoError(t, err)
		args, err := v.args(now, true)
		require.NoError(t, err)
		require.Equal(t, "20600102030405Z", args[3])
	})

	daysCases := []struct {
		name     string
		lifetime time.Duration
		days     int
	}{
		{"sub-day", 23 * time.Hour, 0},
		{"exact day", 24 * time.Hour, 0},
		{"day plus margin minus clock fraction", 24*time.Hour + tongsuoDaysModeSafetyMargin, 0},
		{"day plus margin", 24*time.Hour + tongsuoDaysModeSafetyMargin + time.Second, 1},
		{"just below two days plus margin", 48*time.Hour + tongsuoDaysModeSafetyMargin - time.Second, 1},
		{"multi-day", 72*time.Hour + 17*time.Minute, 3},
	}
	for _, tc := range daysCases {
		t.Run("days mode "+tc.name, func(t *testing.T) {
			t.Parallel()
			v, err := newTongsuoValidity(now, now, now.Add(tc.lifetime))
			require.NoError(t, err)
			args, err := v.args(now, false)
			if tc.days == 0 {
				require.Error(t, err)
				require.Nil(t, args)
				return
			}
			require.NoError(t, err)
			require.Equal(t, []string{"-days", strconv.Itoa(tc.days)}, args)
			// the tool's window can never pass the request, even with margin delay
			require.False(t, now.Add(tongsuoDaysModeSafetyMargin).
				Add(time.Duration(tc.days)*24*time.Hour).After(now.Add(tc.lifetime)))
		})
	}

	t.Run("days mode rejects future not before", func(t *testing.T) {
		t.Parallel()
		v, err := newTongsuoValidity(now, now.Add(time.Hour), now.Add(96*time.Hour))
		require.NoError(t, err)
		_, err = v.args(now, false)
		require.Error(t, err)
	})

	invalid := []struct {
		name                string
		notBefore, notAfter time.Time
	}{
		{"zero not before", time.Time{}, now.Add(time.Hour)},
		{"zero not after", now, time.Time{}},
		{"past", now.Add(-2 * time.Hour), now.Add(-time.Hour)},
		{"sub-second future truncates to now", now.Add(-time.Hour), now.Add(400 * time.Millisecond)},
		{"inverted", now.Add(2 * time.Hour), now.Add(time.Hour)},
		{"empty", now.Add(time.Hour), now.Add(time.Hour)},
		{"year 10000", now, time.Date(10000, 1, 1, 0, 0, 0, 0, time.UTC)},
	}
	for _, tc := range invalid {
		t.Run("reject "+tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := newTongsuoValidity(now, tc.notBefore, tc.notAfter)
			require.Error(t, err)
		})
	}
}

// TestTongsuoValidityVerifyFailsClosed verifies that the post-issuance check
// rejects certificates whose validity escapes the requested window, which is
// the defense against tool or version rounding differences. Regression for
// issue #40.
func TestTongsuoValidityVerifyFailsClosed(t *testing.T) {
	t.Parallel()
	now := time.Now().UTC().Truncate(time.Second)
	v, err := newTongsuoValidity(now, now, now.Add(time.Hour))
	require.NoError(t, err)

	issued := func(notBefore, notAfter time.Time) []byte {
		return newGoFixtureCertForTest(t, &x509.Certificate{
			Subject:   pkix.Name{CommonName: "validity-verify"},
			NotBefore: notBefore,
			NotAfter:  notAfter,
		}, nil)
	}

	require.NoError(t, verifyTongsuoIssuedCert(issued(now, now.Add(time.Hour)),
		tongsuoIssuanceExpectation{validity: &v}))
	require.Error(t, verifyTongsuoIssuedCert(issued(now, now.Add(25*time.Hour)),
		tongsuoIssuanceExpectation{validity: &v}), "later NotAfter must be rejected")
	require.Error(t, verifyTongsuoIssuedCert(issued(now.Add(-time.Hour), now.Add(time.Hour)),
		tongsuoIssuanceExpectation{validity: &v}), "earlier NotBefore must be rejected")
}

// TestTongsuoLegacyDaysValidityNeverExtends forces the whole-day fallback used
// for binaries without -not_after against the real tongsuo binary and verifies
// that issued certificates never outlive the request, while windows that
// cannot be represented are rejected. Regression for issue #40.
func TestTongsuoLegacyDaysValidityNeverExtends(t *testing.T) {
	t.Parallel()
	ins := newSecurityTestTongsuo(t)
	require.True(t, ins.exactValidity, "Tongsuo 8.5 must be detected as supporting -not_after")
	ins.exactValidity = false // this instance is private to the test
	ctx := t.Context()

	caKey, caDer := newTongsuoSM2CAForTest(t, ins, "legacy-days-root")
	csrDer := newTongsuoSM2CSRForTest(t, ins, WithX509CSRCommonName("legacy-days-leaf"))

	for _, lifetime := range []time.Duration{50 * time.Hour, 72*time.Hour + 17*time.Minute} {
		now := time.Now().UTC()
		notAfter := now.Add(lifetime)

		selfDer, err := ins.NewX509Cert(ctx, caKey,
			WithX509CertCommonName("legacy-days-self"),
			WithX509CertNotBefore(now), WithX509CertNotAfter(notAfter))
		require.NoError(t, err)

		leafDer, err := ins.NewX509CertByCSR(ctx, caDer, caKey, csrDer,
			WithX509SignCSRNotBefore(now), WithX509SignCSRNotAfter(notAfter))
		require.NoError(t, err)

		for _, der := range [][]byte{selfDer, leafDer} {
			cert := parseSMCertForTest(t, der)
			require.False(t, cert.NotAfter.After(notAfter))
			require.True(t, cert.NotAfter.After(notAfter.Add(-24*time.Hour-tongsuoDaysModeSafetyMargin-time.Minute)))
		}
	}

	for _, lifetime := range []time.Duration{time.Hour, 24 * time.Hour} {
		now := time.Now().UTC()
		certDer, err := ins.NewX509CertByCSR(ctx, caDer, caKey, csrDer,
			WithX509SignCSRNotBefore(now), WithX509SignCSRNotAfter(now.Add(lifetime)))
		require.Error(t, err, "lifetime %s cannot be encoded in whole days", lifetime)
		require.Nil(t, certDer)
	}
}
