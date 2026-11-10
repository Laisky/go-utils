package securehttp

import (
	"net/http"
	"strings"

	"github.com/Laisky/errors/v2"
)

// StrictEndpoint validates raw as an explicit HTTPS URL without userinfo.
// It returns the normalized endpoint or a safe error, without making requests.
func StrictEndpoint(raw string) (string, error) {
	if !strings.Contains(strings.TrimSpace(raw), "://") {
		return "", errors.New("endpoint must be an explicit HTTPS URL")
	}
	u, err := ParseEndpoint(raw, false)
	if err != nil {
		return "", errors.Wrap(err, "validate secure endpoint")
	}
	if u.Fragment != "" {
		return "", errors.New("endpoint must not contain a fragment")
	}
	return u.String(), nil
}

// CloneWithoutRedirects shallow-copies client and rejects every redirect before
// transmission. A nil client selects net/http defaults. The transport and jar
// remain shared: callers must trust their implementations and must not mutate
// them concurrently. Custom transports are responsible for TLS verification,
// honoring context cancellation, and not following redirects themselves.
func CloneWithoutRedirects(client *http.Client) *http.Client {
	if client == nil {
		client = &http.Client{}
	}
	cloned := *client
	cloned.CheckRedirect = rejectRedirect
	return &cloned
}

// rejectRedirect refuses req regardless of via, before headers or body are sent.
func rejectRedirect(_ *http.Request, _ []*http.Request) error {
	return errors.New("redirect refused for credential-bearing request")
}
