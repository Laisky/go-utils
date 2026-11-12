package log

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"reflect"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestLogCallerRedirectPolicy verifies standard redirect replay and caller refusal,
// without treating either application-selected policy as an SDK vulnerability.
func TestLogCallerRedirectPolicy(t *testing.T) {
	for _, status := range []int{307, 308} {
		for _, policy := range []string{"default", "allow", "reject"} {
			reject := policy == "reject"
			t.Run(fmt.Sprintf("%d/policy=%s", status, policy), func(t *testing.T) {
				var hits, decisions atomic.Int32
				received := make(chan []byte, 2)
				sink := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					hits.Add(1)
					b, err := io.ReadAll(r.Body)
					require.NoError(t, err)
					require.Equal(t, http.MethodPost, r.Method)
					if hits.Load() == 1 {
						require.Equal(t, "header-fixture", r.Header.Get("X-API-Key"))
					}
					received <- b
					_, err = io.WriteString(w, `{"data":{"TelegramMonitorAlert":{"name":"ok"}}}`)
					require.NoError(t, err)
				}))
				defer sink.Close()
				origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, sink.URL, status) }))
				defer origin.Close()
				client := &http.Client{Timeout: time.Second, CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
					decisions.Add(1)
					if reject {
						return http.ErrUseLastResponse
					}
					return nil
				}}
				if policy == "default" {
					client = nil
				}
				sender := NewPusherHTTPSender(client, origin.URL, map[string]string{"X-API-Key": "header-fixture"})
				err := sender.Send(context.Background(), []byte("payload"))
				if reject {
					require.Error(t, err)
				} else {
					require.NoError(t, err)
					require.Equal(t, "payload", string(<-received))
				}
				alert, err := NewAlert(context.Background(), origin.URL, WithAlertHTTPClient(client), WithAlertType("kind"), WithAlertToken("synthetic-token"))
				require.NoError(t, err)
				// Synchronous GraphQL call uses the same configured client as the worker.
				err = alert.cli.Mutate(context.Background(), new(alertMutation), map[string]any{"type": "kind", "token": "synthetic-token", "msg": "payload"})
				alert.Close()
				<-alert.Done()
				if reject {
					require.Error(t, err)
					require.Zero(t, hits.Load())
				} else {
					require.NoError(t, err)
					var b map[string]any
					require.NoError(t, json.Unmarshal(<-received, &b))
					require.Equal(t, map[string]any{"type": "kind", "token": "synthetic-token", "msg": "payload"}, b["variables"])
					require.EqualValues(t, 2, hits.Load())
				}
				if policy == "default" {
					require.Zero(t, decisions.Load())
				} else {
					require.EqualValues(t, 2, decisions.Load())
				}
			})
		}
	}
}

// contractTransport captures requests without changing their transport semantics.
type contractTransport struct {
	roundTrip func(*http.Request) (*http.Response, error)
}

// RoundTrip delegates r and returns the configured response or error.
func (t *contractTransport) RoundTrip(r *http.Request) (*http.Response, error) { return t.roundTrip(r) }

// TestAlertHTTPClientConfiguration verifies transport, cookie jar, timeout and redirect
// configuration are preserved, and the asynchronous GraphQL payload remains stable.
func TestAlertHTTPClientConfiguration(t *testing.T) {
	received := make(chan []byte, 1)
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		received <- b
		require.Equal(t, "fixture=yes", r.Header.Get("Cookie"))
		_, err = io.WriteString(w, `{"data":{"TelegramMonitorAlert":{"name":"ok"}}}`)
		require.NoError(t, err)
	}))
	defer server.Close()
	client := server.Client()
	jar, err := cookiejar.New(nil)
	require.NoError(t, err)
	req, err := http.NewRequest(http.MethodGet, server.URL, nil)
	require.NoError(t, err)
	jar.SetCookies(req.URL, []*http.Cookie{{Name: "fixture", Value: "yes"}})
	client.Jar = jar
	client.Timeout = 2 * time.Second
	client.CheckRedirect = func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }
	transport := client.Transport
	redirect := reflect.ValueOf(client.CheckRedirect).Pointer()
	a, err := NewAlert(context.Background(), server.URL, WithAlertHTTPClient(client), WithAlertPushTimeout(time.Second), WithAlertType("kind"), WithAlertToken("token"))
	require.NoError(t, err)
	defer a.Close()
	require.NoError(t, a.Send("message"))
	select {
	case b := <-received:
		var payload map[string]any
		require.NoError(t, json.Unmarshal(b, &payload))
		require.Equal(t, map[string]any{"type": "kind", "token": "token", "msg": "message"}, payload["variables"])
	case <-time.After(3 * time.Second):
		t.Fatal("delivery timed out")
	}
	a.Close()
	select {
	case <-a.Done():
	case <-time.After(time.Second):
		t.Fatal("shutdown timed out")
	}
	require.Same(t, transport, client.Transport)
	require.Same(t, jar, client.Jar)
	require.Equal(t, 2*time.Second, client.Timeout)
	require.Equal(t, redirect, reflect.ValueOf(client.CheckRedirect).Pointer())
	require.Error(t, a.Send("closed"))
}

// TestAlertHTTPClientCancellation verifies request timeout and Close cancel a custom
// transport without changing the caller's timeout or leaking a worker.
func TestAlertHTTPClientCancellation(t *testing.T) {
	for _, closeEarly := range []bool{false, true} {
		t.Run(fmt.Sprint(closeEarly), func(t *testing.T) {
			started := make(chan struct{})
			finished := make(chan error, 1)
			client := &http.Client{Timeout: time.Hour, Transport: &contractTransport{roundTrip: func(r *http.Request) (*http.Response, error) {
				close(started)
				<-r.Context().Done()
				finished <- r.Context().Err()
				return nil, r.Context().Err()
			}}}
			a, err := NewAlert(context.Background(), "http://fixture.invalid", WithAlertHTTPClient(client), WithAlertPushTimeout(100*time.Millisecond), WithAlertType("kind"), WithAlertToken("token"))
			require.NoError(t, err)
			defer a.Close()
			require.NoError(t, a.Send("message"))
			select {
			case <-started:
			case <-time.After(time.Second):
				t.Fatal("request not started")
			}
			if closeEarly {
				a.Close()
			}
			select {
			case err := <-finished:
				if closeEarly {
					require.ErrorIs(t, err, context.Canceled)
				} else {
					require.ErrorIs(t, err, context.DeadlineExceeded)
				}
			case <-time.After(time.Second):
				t.Fatal("request not canceled")
			}
			a.Close()
			select {
			case <-a.Done():
			case <-time.After(time.Second):
				t.Fatal("worker not stopped")
			}
			require.Equal(t, time.Hour, client.Timeout)
		})
	}
}

// TestAlertNilHTTPClient verifies the optional nil client retains standard defaults.
func TestAlertNilHTTPClient(t *testing.T) {
	a, err := NewAlert(context.Background(), "http://localhost", WithAlertHTTPClient(nil))
	require.NoError(t, err)
	a.Close()
	<-a.Done()
	require.Nil(t, a.httpClient)
}

// TestAlertHTTPClientShorterTimeout verifies a shorter caller timeout is not
// replaced by the alert timeout and the request ends without closing the alert.
func TestAlertHTTPClientShorterTimeout(t *testing.T) {
	finished := make(chan error, 1)
	client := &http.Client{Timeout: 20 * time.Millisecond, Transport: &contractTransport{roundTrip: func(r *http.Request) (*http.Response, error) {
		<-r.Context().Done()
		finished <- r.Context().Err()
		return nil, r.Context().Err()
	}}}
	a, err := NewAlert(context.Background(), "http://fixture.invalid", WithAlertHTTPClient(client), WithAlertPushTimeout(time.Second), WithAlertType("kind"), WithAlertToken("token"))
	require.NoError(t, err)
	defer a.Close()
	require.NoError(t, a.Send("message"))
	select {
	case err := <-finished:
		require.ErrorIs(t, err, context.DeadlineExceeded)
	case <-time.After(500 * time.Millisecond):
		t.Fatal("caller timeout was ignored")
	}
	require.Equal(t, 20*time.Millisecond, client.Timeout)
	a.Close()
	select {
	case <-a.Done():
	case <-time.After(time.Second):
		t.Fatal("worker not stopped")
	}
}
