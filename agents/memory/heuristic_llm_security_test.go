package memory

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"

	"github.com/Laisky/errors/v2"
	"github.com/stretchr/testify/require"
)

const (
	// syntheticLLMKey is a fake bearer credential used only with in-memory transports.
	syntheticLLMKey = "SYNTHETIC_KEY"
	// syntheticLLMBase is a non-resolvable HTTPS API base used by the security tests.
	syntheticLLMBase = "https://agent.example.invalid"
	// heuristicToolResponse is a minimal valid Responses API body for the heuristic tool.
	heuristicToolResponse = `{"output":[{"type":"function_call","name":"extract_and_merge_memories",` +
		`"arguments":"{\"updated_facts\":[],\"deleted_fact_ids\":[]}"}]}`
)

// errLLMRecorderRejected is returned by llmRecordingTransport when no responder
// is configured, so a recorded request is rejected without ever dialing.
var errLLMRecorderRejected = errors.New("recording transport: request rejected without dialing")

// llmRecordedRequest captures the security-relevant parts of one outbound request.
type llmRecordedRequest struct {
	scheme        string
	host          string
	path          string
	authorization string
}

// String renders the recorded request compactly for assertion messages.
func (r llmRecordedRequest) String() string {
	return fmt.Sprintf("%s://%s%s auth=%q", r.scheme, r.host, r.path, r.authorization)
}

// llmRecordingTransport is an in-memory http.RoundTripper that records every
// request and answers through respond, or rejects the request when respond is
// nil. It never opens a socket.
type llmRecordingTransport struct {
	respond func(req *http.Request) *http.Response

	mu   sync.Mutex
	reqs []llmRecordedRequest
}

// RoundTrip records req and returns the scripted response or a rejection.
func (rt *llmRecordingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.Body != nil {
		_, _ = io.Copy(io.Discard, io.LimitReader(req.Body, 1<<20))
		_ = req.Body.Close()
	}

	rt.mu.Lock()
	rt.reqs = append(rt.reqs, llmRecordedRequest{
		scheme:        req.URL.Scheme,
		host:          req.URL.Host,
		path:          req.URL.Path,
		authorization: req.Header.Get("Authorization"),
	})
	rt.mu.Unlock()

	if rt.respond == nil {
		return nil, errLLMRecorderRejected
	}
	return rt.respond(req), nil
}

// requests returns a copy of every request recorded so far.
func (rt *llmRecordingTransport) requests() []llmRecordedRequest {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	return append([]llmRecordedRequest(nil), rt.reqs...)
}

// llmOKResponder answers every request with a valid heuristic tool response.
func llmOKResponder(req *http.Request) *http.Response {
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(heuristicToolResponse)),
		Request:    req,
	}
}

// llmRedirectResponder returns a responder that redirects requests for the
// default responses path to location (307, so the body is replayed) and
// answers any other request with a valid heuristic tool response.
func llmRedirectResponder(location string) func(req *http.Request) *http.Response {
	return func(req *http.Request) *http.Response {
		if req.URL.Path == defaultResponsesPath {
			return &http.Response{
				StatusCode: http.StatusTemporaryRedirect,
				Header:     http.Header{"Location": []string{location}},
				Body:       io.NopCloser(strings.NewReader("")),
				Request:    req,
			}
		}
		return llmOKResponder(req)
	}
}

// syntheticHeuristicInput returns a minimal non-empty heuristic input that
// contains only synthetic data.
func syntheticHeuristicInput() HeuristicFactInput {
	return HeuristicFactInput{
		TurnID:     "turn-synthetic",
		NowRFC3339: "2026-10-07T00:00:00Z",
		InputItems: []ResponseItem{{
			Type:    "message",
			Role:    "user",
			Content: []ResponseContentPart{{Type: "input_text", Text: "synthetic marker"}},
		}},
	}
}

// TestOpenAIResponsesClientRejectsInsecureAPIBase verifies that cleartext,
// malformed, userinfo-bearing, and non-HTTPS API bases are rejected at
// construction and that no credential-bearing request is ever emitted. It is
// the reproduction for issue #47.
func TestOpenAIResponsesClientRejectsInsecureAPIBase(t *testing.T) {
	t.Parallel()

	for _, apiBase := range []string{
		"http://agent.example.invalid",
		"HTTP://agent.example.invalid/v1",
		"http://127.0.0.1:8080",
		"https://user:pass@agent.example.invalid",
		"user:pass@agent.example.invalid",
		"ftp://agent.example.invalid",
		"ws://agent.example.invalid",
		"https://",
		"https://:443",
		"https://agent.example.invalid:99999",
		"https://agent example.invalid",
	} {
		t.Run(apiBase, func(t *testing.T) {
			t.Parallel()

			rec := &llmRecordingTransport{}
			client, err := newOpenAIResponsesClient(openAIResponsesClientConfig{
				APIBase:    apiBase,
				APIKey:     syntheticLLMKey,
				HTTPClient: &http.Client{Transport: rec},
			})
			if err == nil {
				_, _ = client.ExtractAndMergeFacts(context.Background(), syntheticHeuristicInput())
			}
			require.Empty(t, rec.requests(), "credential-bearing request emitted for rejected api base")
			require.Error(t, err, "api base must be rejected at construction")
			require.NotContains(t, err.Error(), "pass@", "errors must not echo URL userinfo")
		})
	}
}

// TestOpenAIResponsesClientAcceptsHTTPSAPIBase verifies that HTTPS API bases
// are accepted and that a scheme-less base is normalized to HTTPS before the
// bearer credential is attached. It is a compatible-path regression for issue
// #47.
func TestOpenAIResponsesClientAcceptsHTTPSAPIBase(t *testing.T) {
	t.Parallel()

	for _, apiBase := range []string{
		syntheticLLMBase,
		"agent.example.invalid",
		"//agent.example.invalid/v1",
		"HTTPS://agent.example.invalid/v1/responses",
	} {
		t.Run(apiBase, func(t *testing.T) {
			t.Parallel()

			rec := &llmRecordingTransport{respond: llmOKResponder}
			client, err := newOpenAIResponsesClient(openAIResponsesClientConfig{
				APIBase:    apiBase,
				APIKey:     syntheticLLMKey,
				HTTPClient: &http.Client{Transport: rec},
			})
			require.NoError(t, err)

			_, err = client.ExtractAndMergeFacts(context.Background(), syntheticHeuristicInput())
			require.NoError(t, err)

			reqs := rec.requests()
			require.Len(t, reqs, 1)
			require.Equal(t, "https", reqs[0].scheme)
			require.Equal(t, "agent.example.invalid", reqs[0].host)
			require.Equal(t, defaultResponsesPath, reqs[0].path)
			require.Equal(t, "Bearer "+syntheticLLMKey, reqs[0].authorization)
		})
	}
}

// TestOpenAIResponsesClientDefaultClientRefusesUnsafeRedirects verifies that
// the default HTTP client refuses HTTPS-to-HTTP downgrades and cross-origin
// redirects, so neither the bearer credential nor the replayed memory payload
// reaches another origin. It is the redirect reproduction for issue #47.
func TestOpenAIResponsesClientDefaultClientRefusesUnsafeRedirects(t *testing.T) {
	t.Parallel()

	for _, location := range []string{
		"http://agent.example.invalid/v1/responses",
		"http://agent.example.invalid:443/v2",
		"https://other.example.invalid/v1/responses",
		"https://evil.agent.example.invalid/v1/responses",
		"https://agent.example.invalid:8443/v1/responses",
	} {
		t.Run(location, func(t *testing.T) {
			t.Parallel()

			client, err := newOpenAIResponsesClient(openAIResponsesClientConfig{
				APIBase: syntheticLLMBase,
				APIKey:  syntheticLLMKey,
			})
			require.NoError(t, err)
			rec := &llmRecordingTransport{respond: llmRedirectResponder(location)}
			client.httpClient.Transport = rec

			_, callErr := client.ExtractAndMergeFacts(context.Background(), syntheticHeuristicInput())
			require.Len(t, rec.requests(), 1, "redirect must not be followed; requests: %v", rec.requests())
			require.Error(t, callErr)
			require.Contains(t, callErr.Error(), "redirect")
		})
	}
}

// TestOpenAIResponsesClientDefaultClientFollowsSameOriginRedirect verifies
// that a redirect staying on the original HTTPS origin is still followed with
// the bearer credential. It is a compatible-path regression for issue #47.
func TestOpenAIResponsesClientDefaultClientFollowsSameOriginRedirect(t *testing.T) {
	t.Parallel()

	client, err := newOpenAIResponsesClient(openAIResponsesClientConfig{
		APIBase: syntheticLLMBase,
		APIKey:  syntheticLLMKey,
	})
	require.NoError(t, err)
	rec := &llmRecordingTransport{respond: llmRedirectResponder("https://agent.example.invalid:443/v2/responses")}
	client.httpClient.Transport = rec

	_, err = client.ExtractAndMergeFacts(context.Background(), syntheticHeuristicInput())
	require.NoError(t, err)

	reqs := rec.requests()
	require.Len(t, reqs, 2)
	for _, req := range reqs {
		require.Equal(t, "https", req.scheme)
		require.Equal(t, "Bearer "+syntheticLLMKey, req.authorization)
	}
}

// TestNewEngineRejectsCleartextLLMAPIBase verifies that the public engine
// constructor refuses a cleartext LLM API base instead of wiring a client
// that would send the API key and memory payloads over HTTP. It is the
// public-API reproduction for issue #47.
func TestNewEngineRejectsCleartextLLMAPIBase(t *testing.T) {
	t.Parallel()

	_, err := NewEngine(newMemoryStorageMock(), Config{
		LLMAPIBase: "http://agent.example.invalid",
		LLMAPIKey:  syntheticLLMKey,
	})
	require.Error(t, err)
}

// TestOpenAIResponsesClientAllowInsecureHTTPOptIn verifies that a cleartext
// API base is only accepted through the explicit opt-in, both on the internal
// client and through the public Config.LLMAllowInsecureHTTP field, and that
// the opt-in does not relax the other URL checks. It is the opt-in regression
// for issue #47.
func TestOpenAIResponsesClientAllowInsecureHTTPOptIn(t *testing.T) {
	t.Parallel()

	t.Run("client accepts http with opt-in", func(t *testing.T) {
		t.Parallel()

		rec := &llmRecordingTransport{respond: llmOKResponder}
		client, err := newOpenAIResponsesClient(openAIResponsesClientConfig{
			APIBase:           "http://127.0.0.1:8080",
			APIKey:            syntheticLLMKey,
			HTTPClient:        &http.Client{Transport: rec},
			AllowInsecureHTTP: true,
		})
		require.NoError(t, err)
		_, err = client.ExtractAndMergeFacts(context.Background(), syntheticHeuristicInput())
		require.NoError(t, err)

		reqs := rec.requests()
		require.Len(t, reqs, 1)
		require.Equal(t, "http", reqs[0].scheme)
		require.Equal(t, defaultResponsesPath, reqs[0].path)
	})

	t.Run("opt-in keeps other checks", func(t *testing.T) {
		t.Parallel()

		for _, apiBase := range []string{"http://user:pass@127.0.0.1", "ftp://127.0.0.1", "http://"} {
			_, err := newOpenAIResponsesClient(openAIResponsesClientConfig{
				APIBase: apiBase, APIKey: syntheticLLMKey, AllowInsecureHTTP: true,
			})
			require.Error(t, err, apiBase)
		}
	})

	t.Run("engine config opt-in", func(t *testing.T) {
		t.Parallel()

		_, err := NewEngine(newMemoryStorageMock(), Config{
			LLMAPIBase:           "http://127.0.0.1:8080",
			LLMAPIKey:            syntheticLLMKey,
			LLMAllowInsecureHTTP: true,
		})
		require.NoError(t, err)
	})

	t.Run("engine rejects loopback http and userinfo without opt-in", func(t *testing.T) {
		t.Parallel()

		for _, apiBase := range []string{"http://127.0.0.1:8080", "http://localhost", "https://user:pass@agent.example.invalid"} {
			_, err := NewEngine(newMemoryStorageMock(), Config{LLMAPIBase: apiBase, LLMAPIKey: syntheticLLMKey})
			require.Error(t, err, apiBase)
		}
	})

	t.Run("engine accepts https", func(t *testing.T) {
		t.Parallel()

		_, err := NewEngine(newMemoryStorageMock(), Config{LLMAPIBase: syntheticLLMBase, LLMAPIKey: syntheticLLMKey})
		require.NoError(t, err)
	})
}
