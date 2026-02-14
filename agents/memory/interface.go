package memory

import (
	"context"
	"time"

	"github.com/Laisky/go-utils/v6/agents/files"
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

// Engine defines standard memory lifecycle hooks for one turn.
type Engine interface {
	BeforeTurn(ctx context.Context, in BeforeTurnInput) (BeforeTurnOutput, error)
	AfterTurn(ctx context.Context, in AfterTurnInput) error
}

// Config controls behavior of standard memory engine.
type Config struct {
	RecentContextItems int
	RecallFactsLimit   int
	SearchLimit        int
	CompactThreshold   float64
	TimeNow            func() time.Time
}

// StandardEngine is a storage-backed implementation of Engine.
type StandardEngine struct {
	storage files.Storage
	conf    Config
}

// NewEngine creates a standard memory engine with pluggable storage backend.
func NewEngine(storage files.Storage, conf Config) (*StandardEngine, error) {
	return newStandardEngine(storage, conf)
}
