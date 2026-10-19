package log

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Laisky/zap"
	"github.com/Laisky/zap/zapcore"
	"github.com/stretchr/testify/require"
)

// defaultAlertMessage receives one actual SDK HTTP alert rather than inspecting selected fields only.
func defaultAlertMessage(t *testing.T, messages <-chan string) string {
	t.Helper()
	select {
	case message := <-messages:
		return message
	case <-time.After(3 * time.Second):
		t.Fatal("default diagnostic alert not delivered")
		return ""
	}
}

// TestAlertDefaultDiagnosticsWithBoundFields reproduces metadata-only alerts from real logger context.
func TestAlertDefaultDiagnosticsWithBoundFields(t *testing.T) {
	a, messages := captureSecurityAlert(t)
	logger, err := New(WithOutputPaths([]string{}), WithZapOptions(zap.HooksWithFields(a.GetZapHook())))
	require.NoError(t, err)
	logger = logger.With(zap.String("task_id", "0199c118-4229-7000-8000-000000000001"), zap.String("worker_category", "future_worker_category"))
	logger.Error("verified crawler task failed", zap.String("stage", "capture"), zap.Int("query_count", 4),
		zap.Int("auth_failure_count", 2), zap.Int("token_count", 12), zap.Int64("duration_ms", 3274),
		zap.Error(fmt.Errorf("capture rendered document: unexpected EOF")))
	message := defaultAlertMessage(t, messages)
	var fields map[string]any
	require.NoError(t, json.Unmarshal([]byte(message[strings.LastIndex(message, "\n")+1:]), &fields))
	require.Equal(t, "0199c118-4229-7000-8000-000000000001", fields["task_id"])
	require.Equal(t, "future_worker_category", fields["worker_category"])
	require.Equal(t, "capture", fields["stage"])
	require.EqualValues(t, 4, fields["query_count"])
	require.EqualValues(t, 2, fields["auth_failure_count"])
	require.EqualValues(t, 12, fields["token_count"])
	require.EqualValues(t, 3274, fields["duration_ms"])
	require.Equal(t, "capture rendered document: unexpected EOF", fields["error"])
}

// TestAlertDiagnosticRedactionCredentialAndPayloadShapes exercises the actual emitted alert envelope.
func TestAlertDiagnosticRedactionCredentialAndPayloadShapes(t *testing.T) {
	a, messages := captureSecurityAlert(t, WithAlertDiagnosticRedaction())
	fields := []zapcore.Field{
		zap.String("stage", "render"), zap.String("category", "future_failure_category"),
		zap.String("serpGoogleApiKey", "SYNTHETIC_API_CREDENTIAL"), zap.String("password", "SYNTHETIC_PASSWORD"),
		zap.String("credential", "SYNTHETIC_CREDENTIAL"), zap.String("cookie_header", "SYNTHETIC_COOKIE_HEADER"),
		zap.String("authorization", "Bearer SYNTHETIC_AUTH_CREDENTIAL"), zap.String("access_token", "SYNTHETIC_ACCESS_CREDENTIAL"),
		zap.String("request_headers", "Cookie: SYNTHETIC_COOKIE_CREDENTIAL"), zap.String("response_body", "SYNTHETIC_USER_BODY"),
		zap.String("request_payload", "SYNTHETIC_USER_PAYLOAD"), zap.String("query", "SYNTHETIC_USER_QUERY"),
		zap.String("target_url", "https://SYNTHETIC_URL_CREDENTIAL@example.test/private?token=SYNTHETIC_QUERY_CREDENTIAL"),
		zap.String("details", "exchange rejected api_key=SYNTHETIC_INLINE_CREDENTIAL; status=403"),
		zap.String("trace_context", "Authorization: Bearer SYNTHETIC_INLINE_AUTH; attempt=2"),
		zap.String("known_token_echo", "configured alert rejected synthetic-auth"),
		zap.Error(fmt.Errorf("fetch https://user:SYNTHETIC_URL_PASSWORD@example.test/private?api_key=SYNTHETIC_URL_KEY failed HTTP 403; response body: SYNTHETIC_ERROR_BODY")),
	}
	entry := zapcore.Entry{Level: zap.ErrorLevel, LoggerName: "worker token=SYNTHETIC_LOGGER_CREDENTIAL", Message: "request failed password=SYNTHETIC_MESSAGE_CREDENTIAL status=403"}
	require.NoError(t, a.GetZapHook()(entry, fields))
	message := defaultAlertMessage(t, messages)
	require.NotContains(t, message, "SYNTHETIC_")
	require.NotContains(t, message, "synthetic-auth")
	require.NotContains(t, message, "example.test")
	require.Contains(t, message, `"stage":"render"`)
	require.Contains(t, message, `"category":"future_failure_category"`)
	require.Contains(t, message, "exchange rejected")
	require.Contains(t, message, "status=403")
	require.Contains(t, message, "HTTP 403")
	require.Contains(t, message, `"error":`)
}

// TestAlertExplicitEmptyFilterRetainsMetadataOnly preserves caller-selected legacy behavior.
func TestAlertExplicitEmptyFilterRetainsMetadataOnly(t *testing.T) {
	a, messages := captureSecurityAlert(t, WithAlertFieldAllowlist())
	require.NoError(t, a.GetZapHook()(zapcore.Entry{Level: zap.ErrorLevel, Message: "metadata only"}, []zapcore.Field{
		zap.String("category", "ordinary failure"), zap.Error(fmt.Errorf("ordinary error")),
	}))
	require.True(t, strings.HasSuffix(defaultAlertMessage(t, messages), "{}"))
}

// panickingDiagnosticError models an error whose formatter must not crash alert delivery.
type panickingDiagnosticError struct{}

// Error panics with a synthetic credential which must not be included in the fallback.
func (panickingDiagnosticError) Error() string { panic("SYNTHETIC_PANIC_CREDENTIAL") }

// TestAlertDiagnosticRedactionComplexFieldsAndNamespace keeps explicitly filtered executable data and scoped fields excluded.
func TestAlertDiagnosticRedactionComplexFieldsAndNamespace(t *testing.T) {
	a, messages := captureSecurityAlert(t, WithAlertDiagnosticRedaction())
	var calls atomic.Int64
	fields := []zapcore.Field{
		zap.String("category", "unrecognized_future_category"),
		zap.Object("nested", alertSecurityObject{&calls}), zap.Array("array", alertSecurityArray{&calls}),
		zap.Any("reflected", map[string]any{"token": "SYNTHETIC_REFLECTED_CREDENTIAL"}),
		zap.Stringer("stringer", alertFieldTypeRecorder{&calls}), zap.Error(panickingDiagnosticError{}),
		zap.Namespace("private"), zap.String("category", "SYNTHETIC_SCOPED_CREDENTIAL"),
	}
	require.NotPanics(t, func() {
		require.NoError(t, a.GetZapHook()(zapcore.Entry{Level: zap.ErrorLevel, Message: "operation failed"}, fields))
	})
	message := defaultAlertMessage(t, messages)
	require.Contains(t, message, "unrecognized_future_category")
	require.Contains(t, message, "error text unavailable")
	require.NotContains(t, message, "SYNTHETIC_")
	require.Zero(t, calls.Load())
}

// TestSanitizeDiagnosticTextRetainsUsefulErrors validates source-log projection without an alert object.
func TestSanitizeDiagnosticTextRetainsUsefulErrors(t *testing.T) {
	for _, tc := range []struct{ raw, useful string }{
		{"navigation failed: unexpected EOF", "unexpected EOF"},
		{"request failed api_key=SYNTHETIC_KEY; HTTP 429", "HTTP 429"},
		{"request failed Authorization: Bearer SYNTHETIC_TOKEN; status=403", "status=403"},
		{"GET https://user:SYNTHETIC_PASSWORD@example.test/private?token=SYNTHETIC_KEY failed: timeout", "timeout"},
		{"conversion failed response body: SYNTHETIC_USER_CONTENT", "conversion failed"},
		{"provider failed payload={\"password\":\"SYNTHETIC_PASSWORD\"}", "provider failed"},
		{"auth_failure_count=3 token_count=42 query_latency=25ms", "auth_failure_count=3 token_count=42 query_latency=25ms"},
	} {
		safe := SanitizeDiagnosticText(tc.raw)
		require.NotContains(t, safe, "SYNTHETIC_")
		require.NotContains(t, safe, "example.test")
		require.Contains(t, safe, tc.useful)
	}
	require.Equal(t, "[diagnostic text exceeds limit]", SanitizeDiagnosticText(strings.Repeat("x", (1<<20)+1)))
}

// TestAlertDefaultSizeCapStillRejectsOversizedDiagnostics preserves bounded queue admission.
func TestAlertDefaultSizeCapStillRejectsOversizedDiagnostics(t *testing.T) {
	a, messages := captureSecurityAlert(t, WithAlertMaxMessageBytes(256))
	require.ErrorIs(t, a.GetZapHook()(zapcore.Entry{Level: zap.ErrorLevel, Message: "failed"}, []zapcore.Field{zap.String("diagnostic", strings.Repeat("x", 257))}), ErrAlertMessageTooLarge)
	require.Empty(t, messages)
	require.Empty(t, a.senderChan)
}
