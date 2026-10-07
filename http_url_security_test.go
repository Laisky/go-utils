package utils

import (
	"bytes"
	"fmt"
	"net/http"
	"net/url"
	"testing"

	"github.com/Laisky/zap"
	"github.com/Laisky/zap/zapcore"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/go-utils/v6/log"
)

// urlSecurityTransport returns URL-bearing failures without contacting a network.
type urlSecurityTransport func(*http.Request) (*http.Response, error)

// RoundTrip delegates to the local deterministic transport.
func (f urlSecurityTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

// TestSecurity49RequestDiagnostics excludes secret markers from request logs and errors.
func TestSecurity49RequestDiagnostics(t *testing.T) {
	const marker = "SYNTHETIC_SECRET_49"
	var output bytes.Buffer
	original := log.Shared
	logger, err := log.New(log.WithOutputPaths([]string{}), log.WithZapOptions(zap.WrapCore(func(zapcore.Core) zapcore.Core {
		return zapcore.NewCore(zapcore.NewJSONEncoder(zap.NewProductionEncoderConfig()), zapcore.AddSync(&output), zap.DebugLevel)
	})))
	require.NoError(t, err)
	log.Shared = logger
	defer func() { log.Shared = original }()
	for _, endpoint := range []string{"https://u:" + marker + "@example.invalid/" + marker + "?token=" + marker, "https://" + marker + "\n.invalid"} {
		client := &http.Client{Transport: urlSecurityTransport(func(*http.Request) (*http.Response, error) {
			return nil, &url.Error{Op: "synthetic", URL: endpoint, Err: fmt.Errorf("%s", marker)}
		})}
		err := RequestJSONWithClient(client, http.MethodGet, endpoint, nil, new(any))
		require.Error(t, err)
		require.NotContains(t, fmt.Sprintf("%+v", err), marker)
	}
	require.NotContains(t, output.String(), marker)
}
