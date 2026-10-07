package crypto

import (
	"context"
	"regexp"
	"strconv"
	"sync"
	"time"

	"github.com/Laisky/errors/v2"
	"github.com/Laisky/zap"
	"github.com/emmansun/gmsm/smx509"

	glog "github.com/Laisky/go-utils/v6/log"
)

const (
	// tongsuoValidityTimeLayout is the UTC layout accepted by the
	// -not_before/-not_after options; a literal "Z" suffix is appended.
	tongsuoValidityTimeLayout = "20060102150405"
	// tongsuoDaysModeSafetyMargin is subtracted from the requested lifetime
	// before whole days are computed for binaries without -not_after, so the
	// delay between capturing the clock and the tool reading its own clock can
	// never push NotAfter past the requested bound.
	tongsuoDaysModeSafetyMargin = 5 * time.Minute
	// tongsuoProbeTimeout bounds each capability probe subprocess.
	tongsuoProbeTimeout = 5 * time.Second
)

// reTongsuoHelpExactValidity matches the -not_before and -not_after options
// in `tongsuo x509 -help` / `tongsuo req -help` output.
var reTongsuoHelpExactValidity = struct{ notBefore, notAfter *regexp.Regexp }{
	notBefore: regexp.MustCompile(`(?m)^\s*-not_before\s`),
	notAfter:  regexp.MustCompile(`(?m)^\s*-not_after\s`),
}

// tongsuoValidityCaps caches the result of the exact validity probe.
type tongsuoValidityCaps struct {
	mu sync.Mutex
	// probed is set once a probe completed and exact holds its result.
	probed bool
	// exact reports whether -not_before/-not_after are supported.
	exact bool
}

// probeExactValidity reports whether both the `x509` and `req` subcommands of
// the binary accept -not_before and -not_after (Tongsuo 8.5 / OpenSSL 3.4+).
// It returns an error only when a probe subprocess cannot be run.
func (t *Tongsuo) probeExactValidity(ctx context.Context) (bool, error) {
	for _, sub := range []string{tongsuoCmdX509, tongsuoCmdReq} {
		probeCtx, cancel := context.WithTimeout(ctx, tongsuoProbeTimeout)
		// `-help` prints its summary to stderr, so inspect both streams.
		stdout, stderr, err := t.runCMDOutputs(probeCtx, []string{sub, "-help"}, nil, nil)
		cancel()
		if err != nil {
			return false, errors.Wrapf(err, "probe `%s -help`", sub)
		}
		out := append(stdout, stderr...)

		if !reTongsuoHelpExactValidity.notBefore.Match(out) ||
			!reTongsuoHelpExactValidity.notAfter.Match(out) {
			glog.Shared.Debug("tongsuo subcommand lacks exact validity options",
				zap.String("subcommand", sub))
			return false, nil
		}
	}

	return true, nil
}

// supportsExactValidity returns the cached exact validity capability,
// probing the binary on first use with a context derived from ctx but detached
// from its cancellation, so one caller's cancellation cannot decide the cached
// result. A probe that cannot run is not cached and reports false, which
// selects the conservative whole-day encoding for that issuance only.
func (t *Tongsuo) supportsExactValidity(ctx context.Context) bool {
	t.validityCaps.mu.Lock()
	defer t.validityCaps.mu.Unlock()

	if t.validityCaps.probed {
		return t.validityCaps.exact
	}

	exact, err := t.probeExactValidity(context.WithoutCancel(ctx))
	if err != nil {
		glog.Shared.Debug("tongsuo exact validity probe failed", zap.Error(err))
		return false
	}

	t.validityCaps.probed = true
	t.validityCaps.exact = exact
	glog.Shared.Debug("tongsuo exact validity support detected", zap.Bool("exact", exact))
	return exact
}

// tongsuoValidity is a validated certificate validity window at the
// one-second precision of X.509 time encodings.
type tongsuoValidity struct {
	// notBefore is the requested start, in UTC, truncated to whole seconds.
	notBefore time.Time
	// notAfter is the requested end, in UTC, truncated to whole seconds so it
	// never exceeds the request.
	notAfter time.Time
}

// newTongsuoValidity validates a requested validity window against one
// captured clock value now. It rejects zero bounds, windows that already
// ended (NotAfter <= now), empty or inverted windows (NotAfter <= NotBefore)
// and years outside 0001-9999, all before anything is issued. Both bounds are
// truncated to whole seconds, which never extends NotAfter.
func newTongsuoValidity(now, notBefore, notAfter time.Time) (tongsuoValidity, error) {
	if notBefore.IsZero() {
		return tongsuoValidity{}, errors.New("not before must be set")
	}
	if notAfter.IsZero() {
		return tongsuoValidity{}, errors.New("not after must be set")
	}

	v := tongsuoValidity{
		notBefore: notBefore.UTC().Truncate(time.Second),
		notAfter:  notAfter.UTC().Truncate(time.Second),
	}
	for _, bound := range []time.Time{v.notBefore, v.notAfter} {
		if bound.Year() < 1 || bound.Year() > 9999 {
			return tongsuoValidity{}, errors.Errorf("validity bound %s is out of range", bound)
		}
	}
	if !v.notAfter.After(now) {
		return tongsuoValidity{}, errors.Errorf(
			"not after %s must be later than the current time %s",
			v.notAfter.Format(time.RFC3339), now.UTC().Format(time.RFC3339))
	}
	if !v.notAfter.After(v.notBefore) {
		return tongsuoValidity{}, errors.Errorf("not after %s must be later than not before %s",
			v.notAfter.Format(time.RFC3339), v.notBefore.Format(time.RFC3339))
	}

	return v, nil
}

// args returns the tongsuo options that encode the window. With exact set it
// returns -not_before/-not_after with the exact UTC bounds. Otherwise it
// returns a conservative whole-day -days value computed against now minus a
// safety margin (the tool then starts the window at its own clock), and an
// error when the request cannot be represented without extending NotAfter or
// starting earlier than a future NotBefore.
func (v tongsuoValidity) args(now time.Time, exact bool) ([]string, error) {
	if exact {
		return []string{
			"-not_before", v.notBefore.Format(tongsuoValidityTimeLayout) + "Z",
			"-not_after", v.notAfter.Format(tongsuoValidityTimeLayout) + "Z",
		}, nil
	}

	if v.notBefore.After(now) {
		return nil, errors.Errorf("not before %s is in the future, which this tongsuo "+
			"binary cannot encode without -not_before", v.notBefore.Format(time.RFC3339))
	}

	days := int((v.notAfter.Sub(now) - tongsuoDaysModeSafetyMargin) / (24 * time.Hour))
	if days < 1 {
		return nil, errors.Errorf("not after %s cannot be encoded in whole days without "+
			"extending it, and this tongsuo binary lacks -not_after", v.notAfter.Format(time.RFC3339))
	}

	return []string{"-days", strconv.Itoa(days)}, nil
}

// verify requires that the issued certificate's validity lies within the
// requested window: NotAfter never later than requested, NotBefore never
// earlier than requested, and a non-empty window. It returns an error
// describing the first violation.
func (v tongsuoValidity) verify(cert *smx509.Certificate) error {
	if cert.NotAfter.After(v.notAfter) {
		return errors.Errorf("issued certificate expires at %s, after the requested %s",
			cert.NotAfter.UTC().Format(time.RFC3339), v.notAfter.Format(time.RFC3339))
	}
	if cert.NotBefore.Before(v.notBefore) {
		return errors.Errorf("issued certificate starts at %s, before the requested %s",
			cert.NotBefore.UTC().Format(time.RFC3339), v.notBefore.Format(time.RFC3339))
	}
	if !cert.NotAfter.After(cert.NotBefore) {
		return errors.Errorf("issued certificate has an empty validity window")
	}

	return nil
}

// validityArgs captures one clock value, validates the requested window and
// returns the validated window together with the tongsuo options encoding it.
// ctx bounds the one-time capability probe. It returns an error before any
// issuance when the window is invalid or cannot be represented by this binary
// without extending it.
func (t *Tongsuo) validityArgs(ctx context.Context, notBefore, notAfter time.Time) (
	tongsuoValidity, []string, error) {
	now := time.Now().UTC()
	v, err := newTongsuoValidity(now, notBefore, notAfter)
	if err != nil {
		return tongsuoValidity{}, nil, errors.Wrap(err, "invalid validity window")
	}

	exact := t.supportsExactValidity(ctx)
	args, err := v.args(now, exact)
	if err != nil {
		return tongsuoValidity{}, nil, errors.Wrap(err, "unrepresentable validity window")
	}

	glog.Shared.Debug("tongsuo validity window",
		zap.Time("not_before", v.notBefore),
		zap.Time("not_after", v.notAfter),
		zap.Bool("exact", exact))
	return v, args, nil
}
