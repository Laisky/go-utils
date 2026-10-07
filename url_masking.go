package utils

import (
	"net/url"
	"strings"

	"github.com/Laisky/errors/v2"
)

// MaskURLPassword replaces only the password in a hierarchical URL's userinfo.
// Absolute and protocol-relative URLs with a host are supported. Mask is literal
// data, not a replacement template. Path, query, and fragment secrets are NOT
// redacted. Parse failures return a bounded error that never includes the URL.
func MaskURLPassword(rawURL, mask string) (string, error) {
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Host == "" || parsed.Opaque != "" {
		return "", errors.New("mask URL password: invalid hierarchical URL")
	}
	if parsed.User != nil {
		if _, hasPassword := parsed.User.Password(); hasPassword {
			parsed.User = url.UserPassword(parsed.User.Username(), mask)
			// '*' is legal userinfo data. Retain the legacy readable star mask
			// without unescaping reserved bytes or applying a regex template.
			encoded := parsed.User.String()
			separator := strings.LastIndexByte(encoded, ':')
			display := encoded[:separator+1] + strings.ReplaceAll(encoded[separator+1:], "%2A", "*")
			return strings.Replace(parsed.String(), encoded+"@", display+"@", 1), nil
		}
	}
	return parsed.String(), nil
}

// URLMasking masks a URL userinfo password, treating mask literally. Invalid URLs
// return a fixed redacted marker rather than potentially exposing credentials.
// It does not redact secrets in the path, query, fragment, or username.
// Use MaskURLPassword when the caller needs to distinguish malformed input.
func URLMasking(rawURL, mask string) string {
	masked, err := MaskURLPassword(rawURL, mask)
	if err != nil {
		return "[invalid URL]"
	}
	return masked
}
