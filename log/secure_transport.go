package log

import (
	"context"
	"net/http"

	"github.com/Laisky/errors/v2"

	"github.com/Laisky/go-utils/v6/internal/securehttp"
)

// NewSecurePusherHTTPSender returns a sender that requires an explicit HTTPS
// endpoint without userinfo and refuses every redirect before transmission.
// Headers and client configuration are copied; the transport and jar remain
// shared. Custom transports must verify TLS, honor context, and never redirect
// internally. The caller's CheckRedirect is not used or modified.
func NewSecurePusherHTTPSender(client *http.Client, endpoint string, headers map[string]string) (*PusherHTTPSender, error) {
	endpoint, err := securehttp.StrictEndpoint(endpoint)
	if err != nil {
		return nil, errors.Wrap(err, "create secure log sender")
	}
	return NewPusherHTTPSender(securehttp.CloneWithoutRedirects(client), endpoint, headers), nil
}

// NewSecureAlert returns an Alert using the existing GraphQL payload and lifecycle
// with an explicit HTTPS endpoint without userinfo and no redirects. It clones
// client configuration (nil selects net/http defaults), replacing CheckRedirect.
// The shared transport and jar must be trusted: custom transports must verify TLS,
// honor context, and never redirect internally. WithAlertPushTimeout still bounds
// every request; any shorter client timeout also applies. No worker starts on error.
func NewSecureAlert(ctx context.Context, endpoint string, client *http.Client, opts ...AlertOption) (*Alert, error) {
	endpoint, err := securehttp.StrictEndpoint(endpoint)
	if err != nil {
		return nil, errors.Wrap(err, "create secure alert")
	}
	return newAlert(ctx, endpoint, securehttp.CloneWithoutRedirects(client), opts...)
}
