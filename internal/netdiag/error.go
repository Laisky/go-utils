// Package netdiag renders endpoint failures without credential-bearing URL data.
package netdiag

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
)

// Endpoint returns only an HTTP scheme and authority, never userinfo, path,
// query, or fragment. Malformed and unsupported URLs fail closed.
func Endpoint(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.Opaque != "" || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return "[invalid endpoint]"
	}
	return u.Scheme + "://" + u.Host
}

// Error retains classification privately without exposing a printable cause chain.
// Inspecting a cause with errors.As is an explicit diagnostic operation; callers
// must not subsequently log the extracted original URL-bearing error.
type Error struct {
	message string
	cause   error
}

// New returns a safe endpoint error. Operation must be a fixed, nonsecret label.
func New(operation, endpoint string, cause error) error {
	suffix := ""
	switch {
	case errors.Is(cause, context.DeadlineExceeded):
		suffix = " (deadline exceeded)"
	case errors.Is(cause, context.Canceled):
		suffix = " (canceled)"
	}
	return &Error{message: operation + " at " + Endpoint(endpoint) + suffix, cause: cause}
}

// Error returns only safe text; the raw underlying message is intentionally omitted.
func (e *Error) Error() string { return e.message }

// Format keeps all standard formatting verbs, including %#v, on the safe surface.
func (e *Error) Format(s fmt.State, _ rune) { _, _ = io.WriteString(s, e.message) }

// Is preserves sentinel matching without adding the private cause to Unwrap chains.
func (e *Error) Is(target error) bool { return errors.Is(e.cause, target) }

// As preserves explicit typed-error inspection without automatic error-chain rendering.
func (e *Error) As(target any) bool { return errors.As(e.cause, target) }

// Timeout retains the standard network timeout classification.
func (e *Error) Timeout() bool {
	var n net.Error
	return errors.As(e.cause, &n) && n.Timeout()
}
