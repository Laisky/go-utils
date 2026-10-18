package log

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/Laisky/zap"
	"github.com/stretchr/testify/require"
)

// TestAlertExplicitFilterRedactsSecretsWithoutChangingSelection preserves filtered diagnostics without credential output.
func TestAlertExplicitFilterRedactsSecretsWithoutChangingSelection(t *testing.T) {
	alert, messages := captureSecurityAlert(t, WithAlertFieldAllowlist("status", "details", "password", "request_body", "retry_count"))
	logger, err := New(WithOutputPaths([]string{}), WithZapOptions(zap.HooksWithFields(alert.GetZapHook())))
	require.NoError(t, err)
	logger = logger.With(zap.String("status", "degraded"), zap.Int("retry_count", 2))
	logger.Error("upstream failed token=SYNTHETIC_MESSAGE_CREDENTIAL", zap.String("details", "upstream HTTP 503 api_key=SYNTHETIC_INLINE_CREDENTIAL; retry budget exhausted"),
		zap.String("password", "SYNTHETIC_PASSWORD"), zap.String("request_body", "SYNTHETIC_BODY"), zap.Int("unselected", 8), zap.Error(fmt.Errorf("legacy filtered error")))
	message := defaultAlertMessage(t, messages)
	require.NotContains(t, message, "SYNTHETIC_")
	require.Contains(t, message, "HTTP 503")
	require.Contains(t, message, "retry budget exhausted")
	var fields map[string]any
	require.NoError(t, json.Unmarshal([]byte(message[strings.LastIndex(message, "\n")+1:]), &fields))
	require.Equal(t, "degraded", fields["status"])
	require.EqualValues(t, 2, fields["retry_count"])
	require.Equal(t, diagnosticRedacted, fields["password"])
	require.Equal(t, diagnosticRedacted, fields["request_body"])
	require.NotContains(t, fields, "unselected")
	require.NotContains(t, fields, "error")
}

// TestAlertDiagnosticRedactionTargetedPayloadLabels retains measurements while dropping sensitive payload labels.
func TestAlertDiagnosticRedactionTargetedPayloadLabels(t *testing.T) {
	alert, messages := captureSecurityAlert(t, WithAlertDiagnosticRedaction())
	logger, err := New(WithOutputPaths([]string{}), WithZapOptions(zap.HooksWithFields(alert.GetZapHook())))
	require.NoError(t, err)
	logger.Error("render failed", zap.String("raw_html", "SYNTHETIC_HTML_BODY"), zap.String("egress_receipt", "SYNTHETIC_RECEIPT_BODY"),
		zap.String("raw_response", "SYNTHETIC_RESPONSE_BODY"), zap.String("api_key_v2", "SYNTHETIC_API_CREDENTIAL"),
		zap.Int("html_length", 24), zap.Bool("receipt_verified", true), zap.Int("receipt_version", 1), zap.Int("raw_body_bytes", 4), zap.Int("auth_failure_count_v2", 2))
	message := defaultAlertMessage(t, messages)
	require.NotContains(t, message, "SYNTHETIC_")
	for _, name := range []string{"html_length", "receipt_verified", "receipt_version", "raw_body_bytes", "auth_failure_count_v2"} {
		require.Contains(t, message, name)
	}
}
