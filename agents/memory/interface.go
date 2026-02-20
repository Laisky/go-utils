package memory

import (
	"context"
	"time"

	storageengine "github.com/Laisky/go-utils/v6/agents/memory/storage"
)

// ResponseItem is a Responses API style item.
type ResponseItem struct {
	Type     string                `json:"type"`
	Role     string                `json:"role,omitempty"`
	Content  []ResponseContentPart `json:"content,omitempty"`
	CallID   string                `json:"call_id,omitempty"`
	Output   string                `json:"output,omitempty"`
	Metadata map[string]string     `json:"metadata,omitempty"`
}

// ResponseContentPart is one content part inside a message item.
type ResponseContentPart struct {
	Type     string `json:"type"`
	Text     string `json:"text,omitempty"`
	ImageURL string `json:"image_url,omitempty"`
	FileID   string `json:"file_id,omitempty"`
	Filename string `json:"filename,omitempty"`
}

// BeforeTurnInput defines engine input before model invocation.
type BeforeTurnInput struct {
	Project          string
	SessionID        string
	UserID           string
	TurnID           string
	CurrentInput     []ResponseItem
	BaseInstructions string
	MaxInputTok      int
}

// BeforeTurnOutput is prepared context payload for model request.
type BeforeTurnOutput struct {
	InputItems        []ResponseItem
	RecallFactIDs     []string
	ContextTokenCount int
}

// AfterTurnInput defines engine input after model response.
type AfterTurnInput struct {
	Project     string
	SessionID   string
	UserID      string
	TurnID      string
	InputItems  []ResponseItem
	OutputItems []ResponseItem
}

// DirectorySummary describes one listed directory and its abstract metadata.
type DirectorySummary struct {
	Path        string
	Abstract    string
	UpdatedAt   string
	HasOverview bool
}

// Engine defines standard memory lifecycle hooks for one turn.
type Engine interface {
	BeforeTurn(ctx context.Context, in BeforeTurnInput) (BeforeTurnOutput, error)
	AfterTurn(ctx context.Context, in AfterTurnInput) error
}

// Management defines optional memory maintenance and directory discovery operations.
type Management interface {
	RunMaintenance(ctx context.Context, project, sessionID string) error
	ListDirWithAbstract(ctx context.Context, project, sessionID, path string, depth, limit int) ([]DirectorySummary, error)
}

// Config controls behavior of standard memory engine.
type Config struct {
	RecentContextItems     int
	RecallFactsLimit       int
	SearchLimit            int
	CompactThreshold       float64
	L1RetentionDays        int
	L2RetentionDays        int
	CompactionMinAge       time.Duration
	SummaryRefreshInterval time.Duration
	MaxProcessedTurns      int
	LLMAPIBase             string
	LLMAPIKey              string
	LLMModel               string
	LLMTimeout             time.Duration
	LLMMaxOutputTokens     int
	HeuristicClient        HeuristicClient
	TimeNow                func() time.Time
}

// StandardEngine is a storage-backed implementation of Engine.
type StandardEngine struct {
	storage   storageengine.Engine
	heuristic HeuristicClient
	conf      Config
}

// HeuristicClient defines model-assisted memory processing for heuristic tasks.
type HeuristicClient interface {
	// ExtractAndMergeFacts extracts key facts from current turn and merges against existing facts.
	// Args:
	//   - ctx: request context.
	//   - in: current turn input and existing facts.
	//
	// Returns:
	//   - []MemoryFact: model-suggested upsert facts.
	//   - error: extraction or merge failure.
	ExtractAndMergeFacts(ctx context.Context, in HeuristicFactInput) ([]MemoryFact, error)
}

// HeuristicFactInput stores payload for model-assisted fact extraction and merge.
type HeuristicFactInput struct {
	TurnID        string
	NowRFC3339    string
	InputItems    []ResponseItem
	ExistingFacts []MemoryFact
}

// NewEngine creates a standard memory engine with pluggable storage backend.
func NewEngine(storage storageengine.Engine, conf Config) (*StandardEngine, error) {
	return newStandardEngine(storage, conf)
}
