package utils

import (
	"context"
	"net/url"
	"os/exec"
	"runtime"
	"strings"

	"github.com/Laisky/errors/v2"
	"github.com/Laisky/zap"

	"github.com/Laisky/go-utils/v6/log"
)

var allowedBrowserURLSchemes = map[string]struct{}{
	"http":   {},
	"https":  {},
	"mailto": {},
}

// OpenURLInDefaultBrowser opens the specified URL in the default browser of the user.
//
// Inspired by https://gist.github.com/sevkin/9798d67b2cb9d07cb05f89f14ba682f8?permalink_comment_id=5019685#gistcomment-5019685
//
//nolint:lll
func OpenURLInDefaultBrowser(ctx context.Context, rawURL string) error {
	parsedURL, err := validateOpenBrowserURL(rawURL)
	if err != nil {
		scheme := ""
		hasHost := false
		if parsedURL != nil {
			scheme = parsedURL.Scheme
			hasHost = parsedURL.Host != ""
		}

		log.Shared.Debug("reject url for default browser",
			zap.String("scheme", scheme),
			zap.Bool("has_host", hasHost),
			zap.Error(err),
		)

		return errors.Wrap(err, "validate url")
	}

	runningInWSL := false
	if runtime.GOOS != "windows" && runtime.GOOS != "darwin" {
		runningInWSL = isWSL(ctx)
	}

	// Security: pass the normalized/parsed URL (not the caller-supplied raw string)
	// to the launcher so the value handed to the OS matches what we validated.
	cmd, args := buildOpenURLCommand(runtime.GOOS, runningInWSL, parsedURL.String())
	log.Shared.Debug("open url in default browser",
		zap.String("scheme", parsedURL.Scheme),
		zap.Bool("has_host", parsedURL.Host != ""),
		zap.String("command", cmd),
		zap.Int("args_len", len(args)),
		zap.Bool("is_wsl", runningInWSL),
	)

	//nolint:gosec // G204: cmd comes from a fixed per-OS allowlist and the URL is validated above.
	launcher := exec.CommandContext(ctx, cmd, args...)
	if err := launcher.Start(); err != nil {
		return errors.Wrapf(err, "start %s", cmd)
	}

	// Reap the launcher once it exits so repeated calls never accumulate zombie
	// processes; the launcher's exit status is informational only.
	go func() {
		if err := launcher.Wait(); err != nil {
			log.Shared.Debug("default browser launcher exited with error",
				zap.String("command", cmd), zap.Error(err))
		}
	}()

	return nil
}

// validateOpenBrowserURL validates URL format and enforces scheme allowlist.
//
// Args:
//   - rawURL: Raw URL string provided by caller.
//
// Returns:
//   - *url.URL: Parsed URL.
//   - error: Validation error when URL is malformed or has disallowed scheme.
func validateOpenBrowserURL(rawURL string) (*url.URL, error) {
	// Security (defense-in-depth): reject any control character (rune < 0x20 or
	// DEL 0x7f). Control bytes such as NUL, CR, LF, or ESC can break argument
	// boundaries or be abused by downstream launchers; URL-legal characters like
	// '&' are intentionally allowed (legitimate query strings contain them).
	for _, r := range rawURL {
		if r < 0x20 || r == 0x7f {
			return nil, errors.Errorf("url contains control character `0x%02x`", r)
		}
	}

	parsedURL, err := url.ParseRequestURI(rawURL)
	if err != nil {
		return nil, errors.Wrap(err, "parse url")
	}

	scheme := strings.ToLower(parsedURL.Scheme)
	if _, ok := allowedBrowserURLSchemes[scheme]; !ok {
		return parsedURL, errors.Errorf("unsupported url scheme `%s`", parsedURL.Scheme)
	}

	if parsedURL.Scheme == "mailto" {
		if parsedURL.Opaque == "" {
			return parsedURL, errors.Errorf("mailto url should contain recipient")
		}

		return parsedURL, nil
	}

	if parsedURL.Host == "" {
		return parsedURL, errors.Errorf("url host should not be empty")
	}

	return parsedURL, nil
}

// buildOpenURLCommand builds the OS-specific command and arguments to open a URL.
//
// Args:
//   - goos: Target operating system name.
//   - runningInWSL: Whether current Linux environment is WSL.
//   - targetURL: Validated URL to open.
//
// Returns:
//   - string: Executable command.
//   - []string: Command arguments.
func buildOpenURLCommand(goos string, runningInWSL bool, targetURL string) (string, []string) {
	switch goos {
	case "windows":
		// Security: do NOT route through `cmd /c start`. Go's os/exec passes args
		// to cmd.exe without neutralizing shell metacharacters (& | < > ^), so a URL
		// containing '&' would let cmd.exe chain an arbitrary command (command
		// injection). rundll32 url.dll,FileProtocolHandler opens http/https/mailto
		// and receives the URL as a single argv element, so metacharacters are inert.
		return "rundll32", []string{"url.dll,FileProtocolHandler", targetURL}
	case "darwin":
		return "open", []string{targetURL}
	default: // "linux", "freebsd", "openbsd", "netbsd"
		if runningInWSL {
			// Security: same cmd.exe metacharacter injection risk as native Windows;
			// use rundll32.exe FileProtocolHandler so the URL stays a single argv element.
			return "rundll32.exe", []string{"url.dll,FileProtocolHandler", targetURL}
		}

		return "xdg-open", []string{targetURL}
	}
}

// isWSL checks if the Go program is running inside Windows Subsystem for Linux
func isWSL(ctx context.Context) bool {
	releaseData, err := exec.CommandContext(ctx, "uname", "-r").Output()
	if err != nil {
		return false
	}

	return strings.Contains(strings.ToLower(string(releaseData)), "microsoft")
}
