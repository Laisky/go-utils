package utils

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestValidateOpenBrowserURL verifies that validateOpenBrowserURL accepts well-formed http, https, and mailto
// URLs (including query strings containing '&') and rejects javascript and file schemes, malformed URLs, an
// https URL without a host, a mailto URL without a recipient, and URLs containing newline, NUL, or DEL bytes.
func TestValidateOpenBrowserURL(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{name: "valid http", input: "http://example.com/path", wantErr: false},
		{name: "valid https", input: "https://example.com/path?q=1", wantErr: false},
		{name: "valid mailto", input: "mailto:test@example.com", wantErr: false},
		{name: "invalid javascript scheme", input: "javascript:alert(1)", wantErr: true},
		{name: "invalid file scheme", input: "file:///etc/passwd", wantErr: true},
		{name: "invalid malformed url", input: "http://[::1", wantErr: true},
		{name: "invalid missing host for https", input: "https:///path", wantErr: true},
		{name: "invalid mailto without recipient", input: "mailto:", wantErr: true},
		// Security: a normal query string with '&' must be accepted (legit URLs use it).
		{name: "valid https with ampersand query", input: "https://example.com/?a=1&b=2", wantErr: false},
		// Security: control characters must be rejected (argument-boundary abuse).
		{name: "invalid url with newline", input: "https://example.com/\npath", wantErr: true},
		{name: "invalid url with null byte", input: "https://example.com/\x00path", wantErr: true},
		{name: "invalid url with del char", input: "https://example.com/\x7fpath", wantErr: true},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := validateOpenBrowserURL(tt.input)
			if tt.wantErr {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, got)
		})
	}
}

// TestBuildOpenURLCommand verifies that buildOpenURLCommand selects rundll32 url.dll,FileProtocolHandler on
// Windows, rundll32.exe on WSL, open on darwin, and xdg-open on Linux, never routes through a shell, and keeps
// a URL containing '&' as a single argv element.
func TestBuildOpenURLCommand(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		goos     string
		isWSL    bool
		url      string
		wantCmd  string
		wantArgs []string
	}{
		{
			// Security: must NOT route through cmd.exe (shell metacharacter injection).
			name:     "windows uses rundll32 FileProtocolHandler",
			goos:     "windows",
			isWSL:    false,
			url:      "https://example.com",
			wantCmd:  "rundll32",
			wantArgs: []string{"url.dll,FileProtocolHandler", "https://example.com"},
		},
		{
			// Security: '&' must stay inside a single argv element, never reach a shell.
			name:     "windows keeps ampersand url as single arg",
			goos:     "windows",
			isWSL:    false,
			url:      "https://x/?a=1&calc.exe",
			wantCmd:  "rundll32",
			wantArgs: []string{"url.dll,FileProtocolHandler", "https://x/?a=1&calc.exe"},
		},
		{
			name:     "darwin uses open with url arg",
			goos:     "darwin",
			isWSL:    false,
			url:      "https://example.com",
			wantCmd:  "open",
			wantArgs: []string{"https://example.com"},
		},
		{
			name:     "linux uses xdg-open",
			goos:     "linux",
			isWSL:    false,
			url:      "https://example.com",
			wantCmd:  "xdg-open",
			wantArgs: []string{"https://example.com"},
		},
		{
			// Security: WSL must NOT route through cmd.exe either.
			name:     "wsl uses rundll32.exe FileProtocolHandler",
			goos:     "linux",
			isWSL:    true,
			url:      "https://example.com",
			wantCmd:  "rundll32.exe",
			wantArgs: []string{"url.dll,FileProtocolHandler", "https://example.com"},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			cmd, args := buildOpenURLCommand(tt.goos, tt.isWSL, tt.url)
			require.Equal(t, tt.wantCmd, cmd)
			require.Equal(t, tt.wantArgs, args)
		})
	}
}

// TestOpenURLInDefaultBrowserRejectsInvalidSchemes verifies that OpenURLInDefaultBrowser refuses a
// javascript: URL with an "unsupported url scheme" error instead of launching a browser.
func TestOpenURLInDefaultBrowserRejectsInvalidSchemes(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := OpenURLInDefaultBrowser(ctx, "javascript:alert(1)")
	require.Error(t, err)
	require.Contains(t, err.Error(), "unsupported url scheme")
}

// TestOpenURLInDefaultBrowserRejectsMalformedURL verifies that OpenURLInDefaultBrowser returns a "parse url"
// error for a malformed URL with an unterminated IPv6 host instead of launching a browser.
func TestOpenURLInDefaultBrowserRejectsMalformedURL(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := OpenURLInDefaultBrowser(ctx, "http://[::1")
	require.Error(t, err)
	require.Contains(t, err.Error(), "parse url")
}
