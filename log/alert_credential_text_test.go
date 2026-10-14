package log

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/Laisky/zap"
	"github.com/stretchr/testify/require"
)

// TestAlertCredentialTextRedactionActualReceiver covers credential text in default errors and explicit selected strings.
func TestAlertCredentialTextRedactionActualReceiver(t *testing.T) {
	for _, tc := range []struct{ name, raw, useful string }{
		{"Token auth header", "HTTP 401 exchange rejected; Authorization: Token SYNTHETIC_TOKEN_SCHEME; attempt=2", "HTTP 401"},
		{"Digest auth header", "HTTP 401 exchange rejected; Authorization: Digest username=\"SYNTHETIC_USERNAME\", response=\"SYNTHETIC_SIGNATURE\", nonce=\"SYNTHETIC_NONCE\"", "HTTP 401"},
		{"AWS proxy auth header", "HTTP 403 proxy rejected; Proxy-Authorization: AWS4-HMAC-SHA256 Credential=SYNTHETIC_AWS_CREDENTIAL, SignedHeaders=host;x-amz-date, Signature=SYNTHETIC_AWS_SIGNATURE", "HTTP 403"},
		{"versioned auth token", "navigation failed: auth_token_v2=SYNTHETIC_AUTH_TOKEN; attempt=2", "navigation failed"},
		{"versioned API key", "exchange rejected api_key_v2: SYNTHETIC_API_KEY; HTTP 403", "HTTP 403"},
		{"versioned access token", "exchange rejected access_token_v4 = SYNTHETIC_ACCESS_TOKEN; HTTP 401", "HTTP 401"},
		{"spaced API key", "exchange rejected api key = SYNTHETIC_SPACED_API_KEY; HTTP 403", "HTTP 403"},
		{"spaced client secret", "exchange rejected client secret = 'SYNTHETIC_CLIENT_SECRET'; HTTP 401", "HTTP 401"},
		{"spaced authorization", "exchange rejected Authorization = Bearer   SYNTHETIC_BEARER; attempt=2", "attempt=2"},
		{"Cookie full header", "navigation failed; Cookie: sid=SYNTHETIC_COOKIE_FIRST; session=SYNTHETIC_COOKIE_SECOND\nretry budget exhausted", "retry budget exhausted"},
		{"Set-Cookie full header", "upstream rejected; Set-Cookie: auth=SYNTHETIC_SET_COOKIE; Expires=Wed, 21 Oct 2026 07:28:00 GMT; Path=/SYNTHETIC_USER_PATH; Secure\nHTTP 503", "HTTP 503"},
	} {
		for _, explicit := range []bool{false, true} {
			mode := "default"
			var opts []AlertOption
			if explicit {
				mode = "explicit"
				opts = []AlertOption{WithAlertFieldAllowlist("bound", "details")}
			}
			t.Run(mode+"/"+tc.name, func(t *testing.T) {
				alert, messages := captureSecurityAlert(t, opts...)
				logger, err := New(WithOutputPaths([]string{}), WithZapOptions(zap.HooksWithFields(alert.GetZapHook())))
				require.NoError(t, err)
				logger = logger.With(zap.String("bound", "ordinary-bound-context"))
				logger.Error(tc.raw, zap.String("details", tc.raw), zap.Error(fmt.Errorf("%s", tc.raw)))
				message := defaultAlertMessage(t, messages)
				require.NotContains(t, message, "SYNTHETIC_")
				require.Contains(t, message, tc.useful)
				var fields map[string]any
				require.NoError(t, json.Unmarshal([]byte(message[strings.LastIndex(message, "\n")+1:]), &fields))
				require.Equal(t, "ordinary-bound-context", fields["bound"])
				require.Contains(t, fields["details"], tc.useful)
				if explicit {
					require.NotContains(t, fields, "error", "legacy type filtering remains intact")
				} else {
					require.Contains(t, fields["error"], tc.useful)
				}
			})
		}
	}
}

// TestDiagnosticTextRetainsBodyReadErrorContext distinguishes a read failure from raw payload text.
func TestDiagnosticTextRetainsBodyReadErrorContext(t *testing.T) {
	for _, raw := range []string{"read body: unexpected EOF", "read response body: unexpected EOF"} {
		require.Contains(t, SanitizeDiagnosticText(raw), "unexpected EOF")
	}
	for _, raw := range []string{
		"conversion failed; response body: SYNTHETIC_USER_CONTENT",
		"conversion failed; raw response body: SYNTHETIC_USER_CONTENT",
		"conversion failed; body=SYNTHETIC_USER_CONTENT",
		"conversion failed; request payload: SYNTHETIC_USER_CONTENT",
	} {
		safe := SanitizeDiagnosticText(raw)
		require.Contains(t, safe, "conversion failed")
		require.NotContains(t, safe, "SYNTHETIC_")
	}
	raw := "query_count=4 query_latency=25ms auth_failure_count_v2=2 token_count=12"
	require.Equal(t, raw, SanitizeDiagnosticText(raw))
}
