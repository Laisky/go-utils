package utils

import (
	"context"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestSecurity55ProxySelectionHelper evaluates initialization policy in a fresh process without making network requests.
func TestSecurity55ProxySelectionHelper(t *testing.T) {
	target := os.Getenv("GO_UTILS_SECURITY55_URL")
	if target == "" {
		return
	}
	client := internalHttpCli
	switch os.Getenv("GO_UTILS_SECURITY55_CLIENT") {
	case "explicit":
		var err error
		client, err = NewHTTPClient(WithHTTPClientProxy("http://explicit.example.invalid:8080"))
		require.NoError(t, err)
	case "direct":
		var err error
		client, err = NewHTTPClient()
		require.NoError(t, err)
	}
	transport, ok := client.Transport.(*http.Transport)
	require.True(t, ok)
	req, err := http.NewRequest(http.MethodGet, target, nil)
	require.NoError(t, err)
	if transport.Proxy == nil {
		require.Empty(t, os.Getenv("GO_UTILS_SECURITY55_EXPECT"))
		require.Empty(t, os.Getenv("GO_UTILS_SECURITY55_ERROR"))
		return
	}
	proxy, err := transport.Proxy(req)
	if os.Getenv("GO_UTILS_SECURITY55_ERROR") == "1" {
		require.Error(t, err)
		return
	}
	require.NoError(t, err)
	got := ""
	if proxy != nil {
		got = proxy.String()
	}
	require.Equal(t, os.Getenv("GO_UTILS_SECURITY55_EXPECT"), got)
}

// TestSecurity55EnvironmentProxyPolicy verifies CGI refusal, NO_PROXY rules, scheme selection and explicit configuration isolation.
func TestSecurity55EnvironmentProxyPolicy(t *testing.T) {
	exe, err := os.Executable()
	require.NoError(t, err)
	for _, tc := range []struct {
		name, target, expected, client string
		fail                           bool
		env                            []string
	}{
		{name: "cgi", target: "http://service.example.invalid/", fail: true, env: []string{"REQUEST_METHOD=GET", "HTTP_PROXY=http://upper.example.invalid:8080"}},
		{name: "no-proxy-host", target: "http://service.example.invalid/", env: []string{"HTTP_PROXY=http://upper.example.invalid:8080", "NO_PROXY=service.example.invalid"}},
		{name: "no-proxy-domain", target: "http://api.example.invalid/", env: []string{"HTTP_PROXY=http://upper.example.invalid:8080", "NO_PROXY=.example.invalid"}},
		{name: "no-proxy-ip", target: "http://192.0.2.7/", env: []string{"HTTP_PROXY=http://upper.example.invalid:8080", "NO_PROXY=192.0.2.0/24"}},
		{name: "loopback", target: "http://127.0.0.1/", env: []string{"HTTP_PROXY=http://upper.example.invalid:8080"}},
		{name: "https", target: "https://service.example.invalid/", expected: "http://secure.example.invalid:8080", env: []string{"HTTP_PROXY=http://upper.example.invalid:8080", "HTTPS_PROXY=http://secure.example.invalid:8080"}},
		{name: "lowercase", target: "http://service.example.invalid/", expected: "http://lower.example.invalid:8080", env: []string{"http_proxy=http://lower.example.invalid:8080"}},
		{name: "precedence", target: "http://service.example.invalid/", expected: "http://upper.example.invalid:8080", env: []string{"HTTP_PROXY=http://upper.example.invalid:8080", "http_proxy=http://lower.example.invalid:8080"}},
		{name: "malformed", target: "http://service.example.invalid/", env: []string{"HTTP_PROXY=http://%"}},
		{name: "explicit", target: "http://service.example.invalid/", expected: "http://explicit.example.invalid:8080", client: "explicit", env: []string{"REQUEST_METHOD=GET", "HTTP_PROXY=http://upper.example.invalid:8080", "NO_PROXY=*"}},
		{name: "direct", target: "http://service.example.invalid/", client: "direct", env: []string{"HTTP_PROXY=http://upper.example.invalid:8080"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			child := exec.CommandContext(ctx, exe, "-test.run=^TestSecurity55ProxySelectionHelper$", "-test.timeout=8s")
			for _, item := range os.Environ() {
				key, _, _ := strings.Cut(item, "=")
				switch strings.ToUpper(key) {
				case "HTTP_PROXY", "HTTPS_PROXY", "NO_PROXY", "ALL_PROXY", "REQUEST_METHOD":
					continue
				}
				if strings.HasPrefix(key, "GO_UTILS_SECURITY55_") {
					continue
				}
				child.Env = append(child.Env, item)
			}
			child.Env = append(child.Env, tc.env...)
			child.Env = append(child.Env, "GO_UTILS_SECURITY55_URL="+tc.target, "GO_UTILS_SECURITY55_EXPECT="+tc.expected, "GO_UTILS_SECURITY55_CLIENT="+tc.client)
			if tc.fail {
				child.Env = append(child.Env, "GO_UTILS_SECURITY55_ERROR=1")
			}
			output, err := child.CombinedOutput()
			require.NoError(t, err, string(output))
		})
	}
}
