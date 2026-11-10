package log

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Laisky/graphql"
	"github.com/stretchr/testify/require"
)

// TestLegacyLogRedirectReplay reproduces both credential-bearing legacy clients'
// 307/308 forwarding behavior without changing their compatibility contract.
func TestLegacyLogRedirectReplay(t *testing.T) {
	for _, status := range []int{307, 308} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			received := make(chan []byte, 2)
			headers := make(chan string, 2)
			sink := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body, err := io.ReadAll(r.Body)
				if err != nil {
					t.Error(err)
					return
				}
				received <- body
				headers <- r.Header.Get("X-API-Key")
				_, err = io.WriteString(w, `{"data":{"TelegramMonitorAlert":{"name":"ok"}}}`)
				if err != nil {
					t.Error(err)
				}
			}))
			defer sink.Close()
			origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				http.Redirect(w, r, sink.URL, status)
			}))
			defer origin.Close()
			sender := NewPusherHTTPSender(nil, origin.URL, map[string]string{"X-API-Key": "header-secret"})
			require.NoError(t, sender.Send(context.Background(), []byte("payload-secret")))
			require.Equal(t, "payload-secret", string(<-received))
			require.Equal(t, "header-secret", <-headers)
			alert, err := NewAlert(context.Background(), origin.URL)
			require.NoError(t, err)
			defer alert.Close()
			require.NoError(t, mutateTransportAlert(alert))
			require.Contains(t, string(<-received), "alert-secret")
		})
	}
}

// mutateTransportAlert sends the production GraphQL mutation synchronously so
// transport tests can assert non-delivery after the redirect decision completes.
func mutateTransportAlert(a *Alert) error {
	return a.cli.Mutate(context.Background(), new(alertMutation), map[string]any{
		"type": graphql.String("test"), "token": graphql.String("alert-secret"), "msg": graphql.String("payload-secret"),
	})
}

// TestSecureLogRejectsRedirects checks that no redirected request reaches a sink,
// including same-origin redirects and TLS downgrades, for both secure clients.
func TestSecureLogRejectsRedirects(t *testing.T) {
	for _, status := range []int{301, 302, 303, 307, 308} {
		for _, destination := range []string{"https", "http", "same-origin"} {
			t.Run(fmt.Sprintf("%d/%s", status, destination), func(t *testing.T) {
				var hits atomic.Int32
				handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { hits.Add(1) })
				sink := httptest.NewTLSServer(handler)
				if destination == "http" {
					sink.Close()
					sink = httptest.NewServer(handler)
				}
				defer sink.Close()
				target := sink.URL
				origin := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if r.URL.Path == "/sink" {
						hits.Add(1)
						return
					}
					http.Redirect(w, r, target, status)
				}))
				defer origin.Close()
				if destination == "same-origin" {
					target = origin.URL + "/sink"
				}
				client := origin.Client()
				var callerPolicy atomic.Int32
				client.CheckRedirect = func(_ *http.Request, _ []*http.Request) error { callerPolicy.Add(1); return nil }
				sender, err := NewSecurePusherHTTPSender(client, origin.URL, map[string]string{"X-API-Key": "header-secret"})
				require.NoError(t, err)
				require.Error(t, sender.Send(context.Background(), []byte("payload-secret")))
				alert, err := NewSecureAlert(context.Background(), origin.URL, client)
				require.NoError(t, err)
				defer alert.Close()
				require.Error(t, mutateTransportAlert(alert))
				require.Zero(t, hits.Load())
				require.Zero(t, callerPolicy.Load(), "secure clients must not delegate their policy")
				require.NoError(t, client.CheckRedirect(nil, nil))
				require.EqualValues(t, 1, callerPolicy.Load(), "caller client must remain unchanged")
			})
		}
	}
}

// TestSecureLogEndpointValidation rejects unsafe endpoints before worker creation
// or network activity, while legacy plaintext remains covered by existing tests.
func TestSecureLogEndpointValidation(t *testing.T) {
	for _, endpoint := range []string{"", "http://localhost", "localhost", "//localhost", "https://user:secret@localhost", "https://", "https://localhost:0", "https://localhost:65536", "ftp://localhost", "https://localhost/#secret"} {
		t.Run(endpoint, func(t *testing.T) {
			sender, err := NewSecurePusherHTTPSender(nil, endpoint, nil)
			require.Error(t, err)
			require.Nil(t, sender)
			require.NotContains(t, err.Error(), "secret")
			alert, err := NewSecureAlert(context.Background(), endpoint, nil)
			require.Error(t, err)
			require.Nil(t, alert)
			require.NotContains(t, err.Error(), "secret")
		})
	}
}

// TestSecureLogHTTPSCompatibility verifies unchanged bodies, headers, GraphQL
// fields, asynchronous delivery, and shutdown on a real TLS endpoint.
func TestSecureLogHTTPSCompatibility(t *testing.T) {
	type request struct {
		body []byte
		key  string
	}
	received := make(chan request, 3)
	origin := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Error(err)
			return
		}
		received <- request{body: body, key: r.Header.Get("X-API-Key")}
		_, err = io.WriteString(w, `{"data":{"TelegramMonitorAlert":{"name":"ok"}}}`)
		if err != nil {
			t.Error(err)
		}
	}))
	defer origin.Close()
	headers := map[string]string{"X-API-Key": "header-secret"}
	sender, err := NewSecurePusherHTTPSender(origin.Client(), origin.URL, headers)
	require.NoError(t, err)
	headers["X-API-Key"] = "changed"
	require.NoError(t, sender.Send(context.Background(), []byte("unchanged-body")))
	got := <-received
	require.Equal(t, "unchanged-body", string(got.body))
	require.Equal(t, "header-secret", got.key)
	alert, err := NewSecureAlert(context.Background(), origin.URL, origin.Client(), WithAlertType("test"), WithAlertToken("alert-secret"))
	require.NoError(t, err)
	defer alert.Close()
	require.NoError(t, alert.Send("payload-secret"))
	select {
	case got = <-received:
	case <-time.After(3 * time.Second):
		t.Fatal("alert delivery timed out")
	}
	var body map[string]any
	require.NoError(t, json.Unmarshal(got.body, &body))
	require.Equal(t, map[string]any{"msg": "payload-secret", "token": "alert-secret", "type": "test"}, body["variables"])
	require.Contains(t, body["query"], "TelegramMonitorAlert")
	alert.Close()
	select {
	case <-alert.Done():
	case <-time.After(3 * time.Second):
		t.Fatal("alert shutdown timed out")
	}
	require.Error(t, alert.Send("after close"))
	require.Error(t, sender.Send(nil, []byte("nil context")))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	require.Error(t, sender.Send(ctx, []byte("cancelled")))
}

// TestSecureLogTLSVerification ensures nil clients retain certificate validation
// and canceled alert contexts terminate the worker without admitting payloads.
func TestSecureLogTLSVerification(t *testing.T) {
	var hits atomic.Int32
	origin := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { hits.Add(1) }))
	defer origin.Close()
	sender, err := NewSecurePusherHTTPSender(nil, origin.URL, nil)
	require.NoError(t, err)
	require.Error(t, sender.Send(context.Background(), []byte("secret")))
	alert, err := NewSecureAlert(context.Background(), origin.URL, nil)
	require.NoError(t, err)
	defer alert.Close()
	require.Error(t, mutateTransportAlert(alert))
	require.Zero(t, hits.Load())
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	stopped, err := NewSecureAlert(ctx, origin.URL, origin.Client())
	require.NoError(t, err)
	defer stopped.Close()
	select {
	case <-stopped.Done():
	case <-time.After(3 * time.Second):
		t.Fatal("canceled alert did not terminate")
	}
	require.Error(t, stopped.SendWithType("test", "secret", "message"))
	require.Zero(t, hits.Load())
}
