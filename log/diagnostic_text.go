package log

import (
	"regexp"
	"strings"

	"github.com/Laisky/zap"
	"github.com/Laisky/zap/zapcore"
)

const diagnosticRedacted = "[redacted]"

var (
	diagnosticKeyVersion = regexp.MustCompile(`v[0-9]+$`)
	diagnosticURL        = regexp.MustCompile(`(?i)\b[a-z][a-z0-9+.-]*://[^\s<>"'\x60]+`)
	diagnosticPayload    = regexp.MustCompile(`(?is)\b(?:response[_ -]?(?:body|payload)|` +
		`request[_ -]?(?:body|payload)|body|payload|headers)\s*["']?\s*[:=]\s*.*`)
	diagnosticAuthorization = regexp.MustCompile(`(?i)\b((?:proxy-)?authorization)(?:[_ -]?v[0-9]+)?` +
		`\b["']?\s*[:=]\s*[^\r\n]*`)
	diagnosticSimpleAuthorization = regexp.MustCompile(`(?i)^(?:bearer|basic)\s+[^\s,;"'\]}]+`)
	diagnosticCookie              = regexp.MustCompile(`(?i)\b(?:set-cookie|cookie)["']?\s*[:=]\s*[^\r\n]*`)
	diagnosticCredential          = regexp.MustCompile(`(?i)\b(api[_ -]?key|access[_ -]?key|access[_ -]?token|` +
		`refresh[_ -]?token|id[_ -]?token|auth[_ -]?token|token|password|passwd|pwd|client[_ -]?secret|secret|` +
		`credentials)(?:[_ -]?v[0-9]+)?\b["']?\s*[:=]\s*` +
		`(?:(?:bearer|basic)\s+[^\s,;"'\]}]+|"[^"]*"|'[^']*'|[^\s,;&}\]]+)`)
	diagnosticAuth     = regexp.MustCompile(`(?i)\b(?:bearer|basic)\s+[^\s,;"'\]}]+`)
	diagnosticKeyValue = regexp.MustCompile(`\b(?:sk-(?:proj-|svcacct-)?[A-Za-z0-9_-]{12,}|` +
		`gh[pousr]_[A-Za-z0-9]{20,}|github_pat_[A-Za-z0-9_]{20,}|(?:AKIA|ASIA)[A-Z0-9]{16})\b`)
)

// SanitizeDiagnosticText preserves diagnostic context while redacting recognizable
// credentials, authentication values, URLs and labeled raw payloads. It does not
// inspect arbitrary objects or promise to identify unlabeled personal information;
// callers must not put user content in diagnostic messages. Inputs above 1 MiB
// return a fixed marker rather than allocating redacted copies of oversized text.
func SanitizeDiagnosticText(text string) string {
	if len(text) > 1<<20 {
		return "[diagnostic text exceeds limit]"
	}
	text = diagnosticPayload.ReplaceAllStringFunc(text, func(value string) string {
		at := strings.IndexAny(value, ":=")
		// A fixed transport error is useful context, not a raw response payload.
		if strings.TrimSpace(value[at+1:]) == "unexpected EOF" {
			return value
		}
		return value[:at+1] + " " + diagnosticRedacted
	})
	text = diagnosticCookie.ReplaceAllStringFunc(text, func(value string) string {
		at := strings.IndexAny(value, ":=")
		return value[:at+1] + " " + diagnosticRedacted
	})
	text = diagnosticAuthorization.ReplaceAllStringFunc(text, func(value string) string {
		at := strings.IndexAny(value, ":=")
		raw := strings.TrimLeft(value[at+1:], " \t")
		// Basic/Bearer have one credential; other schemes may contain comma or
		// semicolon-delimited credential parameters, so redact their complete header.
		if matched := diagnosticSimpleAuthorization.FindStringIndex(raw); matched != nil {
			tail := raw[matched[1]:]
			if !strings.HasPrefix(strings.TrimSpace(tail), ",") {
				return value[:at+1] + " " + diagnosticRedacted + tail
			}
		}
		return value[:at+1] + " " + diagnosticRedacted
	})
	text = diagnosticURL.ReplaceAllString(text, "[redacted URL]")
	text = diagnosticCredential.ReplaceAllString(text, "$1="+diagnosticRedacted)
	text = diagnosticAuth.ReplaceAllString(text, "[redacted authentication]")
	return diagnosticKeyValue.ReplaceAllString(text, diagnosticRedacted)
}

// sensitiveDiagnosticKey recognizes credential and raw-payload keys, not an
// allowlist of diagnostic names. Counters such as token_count remain ordinary fields.
func sensitiveDiagnosticKey(key string) bool {
	compact := strings.NewReplacer("_", "", "-", "", ".", "", " ", "").Replace(strings.ToLower(key))
	compact = diagnosticKeyVersion.ReplaceAllString(compact, "")
	switch compact {
	case "query", "querytext", "querystring", "searchquery", "prompt", "content", "body", "payload",
		"headers", "url", "uri", "password", "passwd", "pwd", "secret", "token", "credential",
		"credentials", "cookie", "setcookie":
		return true
	}
	switch compact {
	case "html", "markdown", "rawresponse", "receipt":
		return true
	}
	for _, suffix := range []string{
		"apikey", "apitoken", "accesskey", "accesstoken", "refreshtoken", "idtoken", "authtoken", "pushtoken",
		"clientsecret", "authorization", "authheader", "credential", "credentials", "cookieheader", "bearertoken",
		"authorizationtoken", "sessiontoken", "password", "passwd", "passwordhash", "privatekey",
		"secretkey", "secret", "tokenprefix", "apikeyprefix", "requestheaders", "responseheaders", "requestbody",
		"responsebody", "rawbody", "requestpayload", "responsepayload", "rawpayload",
		"targeturl", "requesturl", "responseurl",
		"rawhtml", "resulthtml", "rawmarkdown", "resultmarkdown", "egressreceipt",
	} {
		if strings.HasSuffix(compact, suffix) {
			return true
		}
	}
	return false
}

// defaultDiagnosticField projects one safe inline scalar or sanitized error
// string. It never invokes object, array, reflection or Stringer marshalers.
func (a *Alert) defaultDiagnosticField(field zapcore.Field) (zapcore.Field, bool) {
	if sensitiveDiagnosticKey(field.Key) || a.sanitizeDiagnosticText(field.Key) != field.Key {
		return zapcore.Field{}, false
	}
	if field.Type == zapcore.ErrorType {
		cause, ok := field.Interface.(error)
		if !ok || cause == nil {
			return zapcore.Field{}, false
		}
		field = zap.String(field.Key, diagnosticErrorText(cause))
	}
	if alertFieldDispositionOf(field.Type) != alertFieldExport {
		return zapcore.Field{}, false
	}
	if field.Type == zapcore.StringType {
		field.String = a.sanitizeDiagnosticText(field.String)
	}
	return field, true
}

// diagnosticErrorText obtains only Error text; panic details and stacks never escape.
func diagnosticErrorText(cause error) (text string) {
	defer func() {
		if recover() != nil {
			text = "[error text unavailable]"
		}
	}()
	return cause.Error()
}

// sanitizeDiagnosticText also removes the configured alert credential if a source echoes it.
func (a *Alert) sanitizeDiagnosticText(text string) string {
	text = SanitizeDiagnosticText(text)
	if a.alertToken != "" {
		text = strings.ReplaceAll(text, a.alertToken, diagnosticRedacted)
	}
	return text
}
