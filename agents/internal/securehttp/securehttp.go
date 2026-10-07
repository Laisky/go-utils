// Package securehttp holds the transport-security policy shared by the
// credential-bearing HTTP clients under agents/ (the MCP JSON-RPC client and
// the heuristic LLM client).
//
// The policy is fail-closed:
//
//   - Endpoints are parsed and validated at construction time, before any
//     request carrying a bearer credential or payload can be built. HTTPS is
//     required, the URL must have a valid host and port, must not carry URL
//     userinfo, and must not be opaque or use any other scheme. A scheme-less
//     value ("host[:port][/path]" or "//host...") is normalized to HTTPS.
//   - Cleartext HTTP is only accepted through an explicit, caller-set insecure
//     opt-in. There is deliberately no implicit loopback or hostname-based
//     exception.
//   - Default HTTP clients only follow redirects that stay on the original
//     request's origin (scheme, host, and port), so an HTTPS-to-HTTP downgrade
//     or a cross-origin hop can never receive the credential or the replayed
//     request body.
package securehttp

import (
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/Laisky/errors/v2"
	"github.com/Laisky/zap"

	"github.com/Laisky/go-utils/v6/log"
)

const (
	schemeHTTPS = "https"
	schemeHTTP  = "http"
	// maxRedirects mirrors net/http's default redirect limit.
	maxRedirects = 10
	// maxEndpointLength bounds the endpoint string accepted for parsing.
	maxEndpointLength = 4096
)

// ParseEndpoint parses and validates the credential-bearing endpoint raw.
//
// Leading and trailing whitespace is ignored and a value without "://" is
// treated as scheme-less and normalized to HTTPS. The scheme must be https,
// or http only when allowInsecureHTTP is true. The URL must not be opaque,
// must not include userinfo, must have a non-empty host name, and any explicit
// port must be in the range 1-65535.
//
// It returns the validated URL, whose scheme is always lower case, or an
// error. Errors never echo the raw endpoint because it may embed credentials
// in its userinfo or query string.
func ParseEndpoint(raw string, allowInsecureHTTP bool) (*url.URL, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil, errors.New("endpoint is required")
	}
	if len(trimmed) > maxEndpointLength {
		return nil, errors.Errorf("endpoint exceeds %d bytes", maxEndpointLength)
	}
	if !strings.Contains(trimmed, "://") {
		trimmed = schemeHTTPS + "://" + strings.TrimPrefix(trimmed, "//")
	}

	parsed, err := url.Parse(trimmed)
	if err != nil {
		// url.Error embeds the full input; only keep its inner cause so URL
		// userinfo or query secrets never reach logs via the returned error.
		var urlErr *url.Error
		if errors.As(err, &urlErr) {
			err = urlErr.Err
		}
		return nil, errors.Wrap(err, "endpoint is not a valid URL")
	}

	switch parsed.Scheme {
	case schemeHTTPS:
	case schemeHTTP:
		if !allowInsecureHTTP {
			return nil, errors.New(
				"endpoint uses cleartext http; https is required unless the explicit insecure http opt-in is enabled")
		}
	default:
		return nil, errors.Errorf("endpoint scheme %q is not supported; https is required", parsed.Scheme)
	}

	if parsed.Opaque != "" {
		return nil, errors.New("endpoint must be a hierarchical URL")
	}
	if parsed.User != nil {
		return nil, errors.New("endpoint must not contain URL userinfo; pass credentials through the API key")
	}
	if parsed.Hostname() == "" {
		return nil, errors.New("endpoint host is required")
	}
	if port := parsed.Port(); port != "" {
		n, convErr := strconv.Atoi(port)
		if convErr != nil || n < 1 || n > 65535 {
			return nil, errors.New("endpoint port must be in the range 1-65535")
		}
	}

	if parsed.Scheme == schemeHTTP {
		log.Shared.Warn("credential-bearing endpoint uses cleartext http by explicit insecure opt-in",
			zap.String("host", parsed.Host))
	}

	return parsed, nil
}

// CheckRedirect is an http.Client CheckRedirect policy for credential-bearing
// clients. The req argument is the pending redirect request and via holds the
// requests already made, oldest first. It returns nil only when req stays on
// the origin (scheme, host, and port) of the original request via[0] and the
// chain is shorter than ten hops; otherwise it returns an error that makes
// http.Client abort before sending req, so neither headers such as
// Authorization nor a replayed body leave the original origin. An HTTPS to
// HTTP downgrade is always refused.
func CheckRedirect(req *http.Request, via []*http.Request) error {
	if len(via) == 0 {
		return nil
	}
	if len(via) >= maxRedirects {
		return errors.Errorf("refusing redirect: stopped after %d redirects", maxRedirects)
	}

	orig := via[0].URL
	if strings.EqualFold(orig.Scheme, schemeHTTPS) && !strings.EqualFold(req.URL.Scheme, schemeHTTPS) {
		return errors.Errorf("refusing redirect that downgrades https to %q", req.URL.Scheme)
	}
	if origin(orig) != origin(req.URL) {
		return errors.Errorf("refusing cross-origin redirect from %s to %s", origin(orig), origin(req.URL))
	}

	return nil
}

// NewHTTPClient returns a new *http.Client for credential-bearing requests
// whose overall request timeout is timeout and whose redirect policy is
// CheckRedirect. It uses http.DefaultTransport, which verifies server
// certificates and requires TLS 1.2 or newer.
func NewHTTPClient(timeout time.Duration) *http.Client {
	return &http.Client{
		Timeout:       timeout,
		CheckRedirect: CheckRedirect,
	}
}

// origin returns the canonical "scheme://host:port" origin of u, lower-casing
// the scheme and host and filling in the scheme's default port, so origins
// that differ only in spelling compare equal.
func origin(u *url.URL) string {
	scheme := strings.ToLower(u.Scheme)
	port := u.Port()
	if port == "" {
		switch scheme {
		case schemeHTTPS:
			port = "443"
		case schemeHTTP:
			port = "80"
		}
	}
	return scheme + "://" + net.JoinHostPort(strings.ToLower(u.Hostname()), port)
}
