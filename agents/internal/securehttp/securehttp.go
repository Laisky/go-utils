// Package securehttp preserves the agent HTTP policy and its insecure-use warning.
package securehttp

import (
	"net/http"
	"net/url"
	"time"

	"github.com/Laisky/zap"

	"github.com/Laisky/go-utils/v6/internal/securehttp"
	"github.com/Laisky/go-utils/v6/log"
)

const maxEndpointLength = 4096
const maxRedirects = 10

// ParseEndpoint validates raw with the shared policy and warns on explicit HTTP use.
// It returns a validated URL or an error without exposing endpoint credentials.
func ParseEndpoint(raw string, allowInsecureHTTP bool) (*url.URL, error) {
	parsed, err := securehttp.ParseEndpoint(raw, allowInsecureHTTP)
	if err != nil {
		return nil, err
	}
	if parsed.Scheme == "http" {
		log.Shared.Warn("credential-bearing endpoint uses cleartext http by explicit insecure opt-in", zap.String("host", parsed.Host))
	}
	return parsed, nil
}

// CheckRedirect returns the shared origin policy's decision for req and via.
func CheckRedirect(req *http.Request, via []*http.Request) error {
	return securehttp.CheckRedirect(req, via)
}

// NewHTTPClient returns the shared credential-bearing client with timeout.
func NewHTTPClient(timeout time.Duration) *http.Client {
	return securehttp.NewHTTPClient(timeout)
}
