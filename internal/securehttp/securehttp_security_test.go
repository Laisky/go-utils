package securehttp

import (
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestParseEndpointPolicy verifies the endpoint policy shared by the
// credential-bearing agent clients: https required, scheme-less values
// normalized to https, http only with the explicit opt-in, and malformed,
// userinfo-bearing, or unsupported endpoints rejected without echoing the raw
// value. It is a regression for issue #47.
func TestParseEndpointPolicy(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		raw       string
		allowHTTP bool
		want      string
		wantErr   string
	}{
		{raw: "https://agent.example.invalid/mcp", want: "https://agent.example.invalid/mcp"},
		{raw: " HTTPS://agent.example.invalid/mcp\n", want: "https://agent.example.invalid/mcp"},
		{raw: "agent.example.invalid", want: "https://agent.example.invalid"},
		{raw: "agent.example.invalid:8443/v1", want: "https://agent.example.invalid:8443/v1"},
		{raw: "//agent.example.invalid/mcp", want: "https://agent.example.invalid/mcp"},
		{raw: "https://[::1]:8443/mcp", want: "https://[::1]:8443/mcp"},
		{raw: "http://127.0.0.1:8080/mcp", allowHTTP: true, want: "http://127.0.0.1:8080/mcp"},
		{raw: "http://agent.example.invalid/mcp", allowHTTP: true, want: "http://agent.example.invalid/mcp"},

		{raw: "", wantErr: "required"},
		{raw: "   ", wantErr: "required"},
		{raw: "http://agent.example.invalid/mcp", wantErr: "cleartext http"},
		{raw: "http://localhost/mcp", wantErr: "cleartext http"},
		{raw: "http://127.0.0.1/mcp", wantErr: "cleartext http"},
		{raw: "ftp://agent.example.invalid", allowHTTP: true, wantErr: "not supported"},
		{raw: "wss://agent.example.invalid", wantErr: "not supported"},
		{raw: "file:///etc/passwd", wantErr: "not supported"},
		{raw: "https://user:secret-pass@agent.example.invalid", wantErr: "userinfo"},
		{raw: "user:secret-pass@agent.example.invalid", wantErr: "userinfo"},
		{raw: "http://user:secret-pass@agent.example.invalid", allowHTTP: true, wantErr: "userinfo"},
		{raw: "https://", wantErr: "host is required"},
		{raw: "https:///mcp", wantErr: "host is required"},
		{raw: "https://:443/mcp", wantErr: "host is required"},
		{raw: "https://agent.example.invalid:0", wantErr: "port"},
		{raw: "https://agent.example.invalid:65536", wantErr: "port"},
		{raw: "https://agent example.invalid", wantErr: "not a valid URL"},
		{raw: "javascript:alert(1)", wantErr: "not a valid URL"},
		{raw: "https://user:secret-pass@agent.example.invalid:bad", wantErr: "not a valid URL"},
	} {
		t.Run(tc.raw, func(t *testing.T) {
			t.Parallel()

			got, err := ParseEndpoint(tc.raw, tc.allowHTTP)
			if tc.wantErr != "" {
				require.Error(t, err)
				require.Nil(t, got)
				require.Contains(t, err.Error(), tc.wantErr)
				require.NotContains(t, err.Error(), "secret-pass", "errors must not echo URL userinfo")
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.want, got.String())
		})
	}

	t.Run("oversized endpoint", func(t *testing.T) {
		t.Parallel()

		long := make([]byte, maxEndpointLength+1)
		for i := range long {
			long[i] = 'a'
		}
		_, err := ParseEndpoint("https://"+string(long), false)
		require.Error(t, err)
	})
}

// newRedirectRequest builds a pending redirect request for rawURL without
// performing any network I/O.
func newRedirectRequest(t *testing.T, rawURL string) *http.Request {
	t.Helper()

	u, err := url.Parse(rawURL)
	require.NoError(t, err)
	return &http.Request{Method: http.MethodPost, URL: u, Header: make(http.Header)}
}

// TestCheckRedirectPolicy verifies that only same-origin redirects are
// followed and that https-to-http downgrades, cross-host, subdomain, and
// cross-port hops are refused, as is an overlong chain. It is a regression
// for issue #47.
func TestCheckRedirectPolicy(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name    string
		orig    string
		next    string
		wantErr string
	}{
		{name: "same origin path change", orig: "https://agent.example.invalid/mcp", next: "https://agent.example.invalid/v2"},
		{name: "explicit default port and case", orig: "https://agent.example.invalid/mcp",
			next: "https://AGENT.Example.invalid:443/v2"},
		{name: "insecure opt-in same origin", orig: "http://127.0.0.1:8080/mcp", next: "http://127.0.0.1:8080/v2"},
		{name: "downgrade same host", orig: "https://agent.example.invalid/mcp",
			next: "http://agent.example.invalid/mcp", wantErr: "downgrades"},
		{name: "downgrade on https port", orig: "https://agent.example.invalid/mcp",
			next: "http://agent.example.invalid:443/mcp", wantErr: "downgrades"},
		{name: "cross host", orig: "https://agent.example.invalid/mcp",
			next: "https://other.example.invalid/mcp", wantErr: "cross-origin"},
		{name: "subdomain", orig: "https://agent.example.invalid/mcp",
			next: "https://evil.agent.example.invalid/mcp", wantErr: "cross-origin"},
		{name: "cross port", orig: "https://agent.example.invalid/mcp",
			next: "https://agent.example.invalid:8443/mcp", wantErr: "cross-origin"},
		{name: "http upgrade is cross origin", orig: "http://127.0.0.1:8080/mcp",
			next: "https://127.0.0.1:8080/mcp", wantErr: "cross-origin"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			via := []*http.Request{newRedirectRequest(t, tc.orig)}
			err := CheckRedirect(newRedirectRequest(t, tc.next), via)
			if tc.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			require.Contains(t, err.Error(), tc.wantErr)
		})
	}

	t.Run("chain is checked against the original origin", func(t *testing.T) {
		t.Parallel()

		via := []*http.Request{
			newRedirectRequest(t, "https://agent.example.invalid/mcp"),
			newRedirectRequest(t, "https://agent.example.invalid/v2"),
		}
		require.Error(t, CheckRedirect(newRedirectRequest(t, "https://other.example.invalid/v3"), via))
	})

	t.Run("redirect limit", func(t *testing.T) {
		t.Parallel()

		via := make([]*http.Request, 0, maxRedirects)
		for range maxRedirects {
			via = append(via, newRedirectRequest(t, "https://agent.example.invalid/mcp"))
		}
		err := CheckRedirect(newRedirectRequest(t, "https://agent.example.invalid/mcp"), via)
		require.Error(t, err)
		require.Contains(t, err.Error(), "redirects")
	})
}

// TestNewHTTPClientInstallsPolicy verifies that the default client used by
// the agent constructors carries the requested timeout and the redirect
// policy. It is a regression for issue #47.
func TestNewHTTPClientInstallsPolicy(t *testing.T) {
	t.Parallel()

	cli := NewHTTPClient(7 * time.Second)
	require.Equal(t, 7*time.Second, cli.Timeout)
	require.NotNil(t, cli.CheckRedirect)

	via := []*http.Request{newRedirectRequest(t, "https://agent.example.invalid/mcp")}
	require.Error(t, cli.CheckRedirect(newRedirectRequest(t, "http://agent.example.invalid/mcp"), via))
}
