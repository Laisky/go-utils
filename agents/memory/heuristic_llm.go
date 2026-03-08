package memory

import (
	"bytes"
	"context"
	"crypto/sha1"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/Laisky/errors/v2"
)

const (
	defaultResponsesPath = "/v1/responses"
	heuristicToolName    = "extract_and_merge_memories"
	maxLLMResponseBytes  = 2 * 1024 * 1024
)

// openAIResponsesClientConfig stores runtime options for heuristic model requests.
type openAIResponsesClientConfig struct {
	APIBase         string
	APIKey          string
	Model           string
	Timeout         time.Duration
	MaxOutputTokens int
	HTTPClient      *http.Client
}

// openAIResponsesClient implements heuristic extraction/merge with Responses API tools.
type openAIResponsesClient struct {
	apiURL          string
	apiKey          string
	model           string
	timeout         time.Duration
	maxOutputTokens int
	httpClient      *http.Client
}

// heuristicToolOutput stores normalized tool payload returned by model.
type heuristicToolOutput struct {
	UpdatedFacts    []MemoryFact `json:"updated_facts"`
	DeletedFactIDs  []string     `json:"deleted_fact_ids"`
	ClassifierNotes string       `json:"classifier_notes"`
}

// openAIResponseRequest stores minimal fields required by Responses API request.
type openAIResponseRequest struct {
	Model           string            `json:"model"`
	Instructions    string            `json:"instructions"`
	Input           string            `json:"input"`
	Tools           []toolSpec        `json:"tools,omitempty"`
	MaxOutputTokens int               `json:"max_output_tokens,omitempty"`
	Metadata        map[string]string `json:"metadata,omitempty"`
}

// toolSpec defines one function tool schema for Responses API.
type toolSpec struct {
	Type        string         `json:"type"`
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	Parameters  map[string]any `json:"parameters,omitempty"`
}

// newOpenAIResponsesClient creates a heuristic client backed by OpenAI-compatible Responses API.
func newOpenAIResponsesClient(conf openAIResponsesClientConfig) (*openAIResponsesClient, error) {
	if strings.TrimSpace(conf.APIBase) == "" {
		return nil, errors.Errorf("api base is required")
	}
	if strings.TrimSpace(conf.APIKey) == "" {
		return nil, errors.Errorf("api key is required")
	}

	model := strings.TrimSpace(conf.Model)
	if model == "" {
		model = defaultLLMModel
	}
	timeout := conf.Timeout
	if timeout <= 0 {
		timeout = defaultLLMTimeout
	}
	maxOutputTokens := conf.MaxOutputTokens
	if maxOutputTokens <= 0 {
		maxOutputTokens = defaultLLMMaxOutputTokens
	}

	httpCli := conf.HTTPClient
	if httpCli == nil {
		httpCli = &http.Client{Timeout: timeout}
	}

	return &openAIResponsesClient{
		apiURL:          normalizeResponsesURL(conf.APIBase),
		apiKey:          conf.APIKey,
		model:           model,
		timeout:         timeout,
		maxOutputTokens: maxOutputTokens,
		httpClient:      httpCli,
	}, nil
}

// ExtractAndMergeFacts runs heuristic extraction/classification/merge and returns memory mutations.
func (client *openAIResponsesClient) ExtractAndMergeFacts(ctx context.Context, in HeuristicFactInput) (HeuristicFactResult, error) {
	inputText := buildHeuristicInputText(in)
	if strings.TrimSpace(inputText) == "" {
		return HeuristicFactResult{}, nil
	}

	reqBody := openAIResponseRequest{
		Model:           client.model,
		Instructions:    memoryHeuristicSystemPrompt(),
		Input:           inputText,
		Tools:           []toolSpec{memoryHeuristicToolSpec()},
		MaxOutputTokens: client.maxOutputTokens,
		Metadata: map[string]string{
			"component": "agents_memory",
			"task":      "extract_merge_facts",
		},
	}

	payload, err := json.Marshal(reqBody)
	if err != nil {
		return HeuristicFactResult{}, errors.Wrap(err, "marshal responses payload")
	}

	ctxWithTimeout, cancel := context.WithTimeout(ctx, client.timeout)
	defer cancel()

	httpReq, err := http.NewRequestWithContext(ctxWithTimeout, http.MethodPost, client.apiURL, bytes.NewReader(payload))
	if err != nil {
		return HeuristicFactResult{}, errors.Wrap(err, "new responses request")
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+client.apiKey)

	httpResp, err := client.httpClient.Do(httpReq)
	if err != nil {
		return HeuristicFactResult{}, errors.Wrap(err, "call responses api")
	}
	defer func() {
		_ = httpResp.Body.Close()
	}()

	respBody, truncated, err := readHTTPBodyWithLimit(httpResp.Body, maxLLMResponseBytes)
	if err != nil {
		return HeuristicFactResult{}, errors.Wrap(err, "read responses body")
	}
	if truncated {
		return HeuristicFactResult{}, errors.Errorf("responses body exceeds limit %d bytes", maxLLMResponseBytes)
	}

	if httpResp.StatusCode < 200 || httpResp.StatusCode >= 300 {
		return HeuristicFactResult{}, errors.Errorf("responses api status=%d body=%s", httpResp.StatusCode, string(respBody))
	}

	output, err := extractHeuristicToolOutput(respBody)
	if err != nil {
		return HeuristicFactResult{}, errors.Wrap(err, "extract heuristic tool output")
	}

	facts := normalizeHeuristicFacts(in.TurnID, in.NowRFC3339, output.UpdatedFacts)
	if len(facts) == 0 && len(output.DeletedFactIDs) == 0 {
		return HeuristicFactResult{}, nil
	}

	return HeuristicFactResult{UpdatedFacts: facts, DeletedFactIDs: deduplicateStrings(output.DeletedFactIDs)}, nil
}

// memoryHeuristicSystemPrompt returns system prompt for extraction/classification/merge tasks.
func memoryHeuristicSystemPrompt() string {
	return strings.Join([]string{
		"You are a memory processing assistant for a chat memory engine.",
		"Your tasks are heuristic and must return structured tool output only.",
		"Task 1: Extract key durable or actionable facts from the current turn.",
		"Task 2: Classify each fact into tiers: L0 permanent identity/preferences, L1 short-term daily, L2 medium-term weekly.",
		"Task 3: Merge with existing facts by preferring newer or more specific values and avoiding duplicates.",
		"Only output facts that should be written into memory.",
		"Keep fact values concise and never include secrets.",
	}, " ")
}

// memoryHeuristicToolSpec returns the function-tool schema for structured heuristic output.
func memoryHeuristicToolSpec() toolSpec {
	return toolSpec{
		Type:        "function",
		Name:        heuristicToolName,
		Description: "Extract key facts, classify memory tier, and merge with existing memory facts.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"updated_facts": map[string]any{
					"type": "array",
					"items": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"fact_id":    map[string]any{"type": "string"},
							"key":        map[string]any{"type": "string"},
							"value":      map[string]any{"type": "string"},
							"tier":       map[string]any{"type": "string", "enum": []string{memoryTierL0, memoryTierL1, memoryTierL2}},
							"confidence": map[string]any{"type": "number"},
						},
						"required": []string{"fact_id", "key", "value", "tier"},
					},
				},
				"deleted_fact_ids": map[string]any{
					"type":  "array",
					"items": map[string]any{"type": "string"},
				},
				"classifier_notes": map[string]any{"type": "string"},
			},
			"required": []string{"updated_facts"},
		},
	}
}

// buildHeuristicInputText builds prompt input including current turn and existing memory facts.
func buildHeuristicInputText(in HeuristicFactInput) string {
	var sb strings.Builder
	sb.WriteString("Current turn input items (JSON):\n")
	inputJSON, _ := json.Marshal(in.InputItems)
	sb.WriteString(string(inputJSON))
	sb.WriteString("\n\nExisting facts (JSON):\n")
	factsJSON, _ := json.Marshal(in.ExistingFacts)
	sb.WriteString(string(factsJSON))

	return sb.String()
}

// normalizeResponsesURL normalizes API base and appends responses path when required.
func normalizeResponsesURL(apiBase string) string {
	trimmed := strings.TrimSpace(apiBase)
	if trimmed == "" {
		return ""
	}

	if !strings.Contains(trimmed, "://") {
		trimmed = "https://" + strings.TrimPrefix(trimmed, "//")
	}

	parsed, err := url.Parse(trimmed)
	if err != nil || parsed.Host == "" {
		trimmed = strings.TrimSuffix(trimmed, "/")
		if strings.HasSuffix(trimmed, defaultResponsesPath) {
			return trimmed
		}

		return trimmed + defaultResponsesPath
	}

	normalizedPath := strings.TrimSuffix(parsed.Path, "/")
	switch {
	case normalizedPath == "":
		parsed.Path = defaultResponsesPath
	case strings.HasSuffix(normalizedPath, defaultResponsesPath):
		parsed.Path = normalizedPath
	case strings.HasSuffix(normalizedPath, "/v1"):
		parsed.Path = normalizedPath + "/responses"
	default:
		parsed.Path = normalizedPath + defaultResponsesPath
	}

	parsed.RawPath = ""
	if parsed.Scheme == "" {
		parsed.Scheme = "https"
	}

	if parsed.RawQuery == "" && parsed.Fragment == "" {
		return parsed.String()
	}

	parsed.RawQuery = ""
	parsed.Fragment = ""
	trimmed = parsed.String()
	if strings.HasSuffix(trimmed, defaultResponsesPath) {
		return trimmed
	}

	return trimmed + defaultResponsesPath
}

// readHTTPBodyWithLimit reads body up to maxBytes and reports whether truncation occurred.
func readHTTPBodyWithLimit(body io.Reader, maxBytes int64) ([]byte, bool, error) {
	respB, err := io.ReadAll(io.LimitReader(body, maxBytes+1))
	if err != nil {
		return nil, false, errors.Wrap(err, "read response body with limit")
	}

	if int64(len(respB)) > maxBytes {
		return respB[:maxBytes], true, nil
	}

	return respB, false, nil
}

// extractHeuristicToolOutput parses responses payload and extracts heuristic tool arguments.
func extractHeuristicToolOutput(respBody []byte) (heuristicToolOutput, error) {
	var parsed map[string]any
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return heuristicToolOutput{}, errors.Wrap(err, "unmarshal responses body")
	}

	argsRaw, found := findToolArguments(parsed, heuristicToolName)
	if !found {
		return heuristicToolOutput{}, errors.Errorf("tool %s not found in responses output", heuristicToolName)
	}

	var output heuristicToolOutput
	if err := json.Unmarshal(argsRaw, &output); err != nil {
		return heuristicToolOutput{}, errors.Wrap(err, "unmarshal heuristic arguments")
	}

	return output, nil
}

// findToolArguments recursively searches response payload and returns tool arguments by tool name.
func findToolArguments(v any, toolName string) ([]byte, bool) {
	switch data := v.(type) {
	case map[string]any:
		if raw, ok := matchToolArgumentsInMap(data, toolName); ok {
			return raw, true
		}
		for _, val := range data {
			if raw, ok := findToolArguments(val, toolName); ok {
				return raw, true
			}
		}
	case []any:
		for _, item := range data {
			if raw, ok := findToolArguments(item, toolName); ok {
				return raw, true
			}
		}
	}

	return nil, false
}

// matchToolArgumentsInMap checks one object for function call fields and serializes arguments.
func matchToolArgumentsInMap(obj map[string]any, toolName string) ([]byte, bool) {
	if name, ok := obj["name"].(string); ok && strings.TrimSpace(name) == toolName {
		if arguments, ok := obj["arguments"]; ok {
			return serializeToolArguments(arguments)
		}
	}

	if functionField, ok := obj["function"].(map[string]any); ok {
		if name, ok := functionField["name"].(string); ok && strings.TrimSpace(name) == toolName {
			if arguments, ok := functionField["arguments"]; ok {
				return serializeToolArguments(arguments)
			}
		}
	}

	return nil, false
}

// serializeToolArguments converts tool argument field into JSON bytes.
func serializeToolArguments(arguments any) ([]byte, bool) {
	switch args := arguments.(type) {
	case string:
		trimmed := strings.TrimSpace(args)
		if trimmed == "" {
			return nil, false
		}
		return []byte(trimmed), true
	default:
		buf, err := json.Marshal(args)
		if err != nil {
			return nil, false
		}
		return buf, true
	}
}

// normalizeHeuristicFacts validates and normalizes model facts for persistence.
func normalizeHeuristicFacts(turnID, nowRFC3339 string, facts []MemoryFact) []MemoryFact {
	normalized := make([]MemoryFact, 0, len(facts))
	for idx, fact := range facts {
		key := strings.TrimSpace(fact.Key)
		value := strings.TrimSpace(fact.Value)
		if key == "" || value == "" {
			continue
		}

		factID := strings.TrimSpace(fact.FactID)
		if factID == "" {
			factID = key
		}

		tier := normalizeFactTier(fact.Tier)
		confidence := fact.Confidence
		if confidence <= 0 || confidence > 1 {
			confidence = 0.78
		}

		id := strings.TrimSpace(fact.ID)
		if id == "" {
			id = buildHeuristicFactID(turnID, factID, idx)
		}

		normalized = append(normalized, MemoryFact{
			ID:           id,
			TS:           nowRFC3339,
			Type:         "fact_upsert",
			FactID:       factID,
			Key:          key,
			Value:        value,
			Confidence:   confidence,
			Tier:         tier,
			SourceTurnID: turnID,
		})
	}

	return deduplicateHeuristicFacts(normalized)
}

// normalizeFactTier normalizes arbitrary tier values into supported memory tiers.
func normalizeFactTier(tier string) string {
	switch strings.ToUpper(strings.TrimSpace(tier)) {
	case memoryTierL0:
		return memoryTierL0
	case memoryTierL1:
		return memoryTierL1
	case memoryTierL2:
		return memoryTierL2
	default:
		return memoryTierL2
	}
}

// buildHeuristicFactID builds deterministic fact id seed for one model-produced fact.
func buildHeuristicFactID(turnID, factID string, idx int) string {
	sum := sha1.Sum([]byte(fmt.Sprintf("%s|%s|%d", turnID, factID, idx)))
	return fmt.Sprintf("%s-llm-%x", turnID, sum[:6])
}

// deduplicateHeuristicFacts keeps latest fact for each fact_id/key pair.
func deduplicateHeuristicFacts(facts []MemoryFact) []MemoryFact {
	if len(facts) == 0 {
		return nil
	}

	type pair struct {
		factID string
		key    string
	}

	latest := make(map[pair]MemoryFact, len(facts))
	order := make([]pair, 0, len(facts))
	for _, fact := range facts {
		p := pair{factID: fact.FactID, key: fact.Key}
		if _, exists := latest[p]; !exists {
			order = append(order, p)
		}
		latest[p] = fact
	}

	result := make([]MemoryFact, 0, len(latest))
	for _, p := range order {
		result = append(result, latest[p])
	}

	sort.SliceStable(result, func(i, j int) bool {
		if result[i].Tier != result[j].Tier {
			return result[i].Tier < result[j].Tier
		}
		if result[i].FactID != result[j].FactID {
			return result[i].FactID < result[j].FactID
		}
		return result[i].Key < result[j].Key
	})

	return result
}
