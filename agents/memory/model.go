package memory

import "time"

const (
	legacyLogFileName         = "log.jsonl"
	legacyContextFileName     = "context.jsonl"
	legacyMemoryFactsFileName = "memory_facts.jsonl"
	legacyMetaFileName        = "meta.json"
)

const (
	memoryTierL0 = "L0"
	memoryTierL1 = "L1"
	memoryTierL2 = "L2"
)

const (
	defaultRecentContextItems     = 30
	defaultRecallFactsLimit       = 20
	defaultSearchLimit            = 5
	defaultCompactThreshold       = 0.8
	defaultL1RetentionDays        = 1
	defaultL2RetentionDays        = 7
	defaultCompactionMinAge       = 24 * time.Hour
	defaultSummaryRefreshInterval = time.Hour
	defaultMaxProcessedTurns      = 1024
	defaultLLMModel               = "openai/gpt-oss-120b"
	defaultLLMTimeout             = 12 * time.Second
	defaultLLMMaxOutputTokens     = 800
)

// MemoryMeta stores session-level memory bookkeeping.
type MemoryMeta struct {
	Version           int      `json:"version"`
	LatestTurnID      string   `json:"latest_turn_id,omitempty"`
	ProcessedTurnIDs  []string `json:"processed_turn_ids,omitempty"`
	LastCompactAt     string   `json:"last_compact_at,omitempty"`
	LastMaintenanceAt string   `json:"last_maintenance_at,omitempty"`
	UpdatedAt         string   `json:"updated_at,omitempty"`
}

// memoryMetaCompat supports backward compatibility for legacy field names.
type memoryMetaCompat struct {
	Version           int      `json:"version"`
	LatestTurnID      string   `json:"latest_turn_id,omitempty"`
	ProcessedTurnIDs  []string `json:"processed_turn_ids,omitempty"`
	ProcessedTurn     []string `json:"processed_turn,omitempty"`
	LastCompactAt     string   `json:"last_compact_at,omitempty"`
	LastMaintenanceAt string   `json:"last_maintenance_at,omitempty"`
	UpdatedAt         string   `json:"updated_at,omitempty"`
}

// LogEvent is an immutable history event stored in log/context files.
type LogEvent struct {
	ID      string       `json:"id"`
	TS      string       `json:"ts"`
	Type    string       `json:"type"`
	TurnID  string       `json:"turn_id,omitempty"`
	Item    ResponseItem `json:"item,omitempty"`
	Summary string       `json:"summary,omitempty"`
}

// MemoryFact is one structured long-term memory fact record.
type MemoryFact struct {
	ID           string  `json:"id"`
	TS           string  `json:"ts"`
	Type         string  `json:"type"`
	FactID       string  `json:"fact_id"`
	Key          string  `json:"key"`
	Value        string  `json:"value"`
	Confidence   float64 `json:"confidence"`
	Tier         string  `json:"tier,omitempty"`
	ExpiresAt    string  `json:"expires_at,omitempty"`
	SourceTurnID string  `json:"source_turn_id,omitempty"`
}

// MemoryPolicy stores persisted retention and maintenance policy.
type MemoryPolicy struct {
	Version    int                   `json:"version"`
	Timezone   string                `json:"timezone"`
	Tiers      map[string]TierPolicy `json:"tiers"`
	Compaction CompactionPolicy      `json:"compaction"`
	Summary    SummaryPolicy         `json:"summary"`
}

// TierPolicy describes retention policy for one memory tier.
type TierPolicy struct {
	RetentionDays int  `json:"retention_days"`
	AutoDelete    bool `json:"auto_delete"`
}

// CompactionPolicy describes compaction controls.
type CompactionPolicy struct {
	Enabled          bool   `json:"enabled"`
	MaxHotShardBytes int64  `json:"max_hot_shard_bytes"`
	MinShardAgeHours int    `json:"min_shard_age_hours"`
	Compression      string `json:"compression"`
}

// SummaryPolicy describes summary generation controls.
type SummaryPolicy struct {
	AbstractWordsMin      int `json:"abstract_words_min"`
	AbstractWordsMax      int `json:"abstract_words_max"`
	OverviewWordsMax      int `json:"overview_words_max"`
	RefreshIntervalMinute int `json:"refresh_interval_minutes"`
}

// defaultMemoryPolicy builds the default persisted policy from runtime config.
func defaultMemoryPolicy(conf Config) MemoryPolicy {
	return MemoryPolicy{
		Version:  1,
		Timezone: "UTC",
		Tiers: map[string]TierPolicy{
			memoryTierL0: {RetentionDays: 0, AutoDelete: false},
			memoryTierL1: {RetentionDays: conf.L1RetentionDays, AutoDelete: true},
			memoryTierL2: {RetentionDays: conf.L2RetentionDays, AutoDelete: true},
		},
		Compaction: CompactionPolicy{
			Enabled:          true,
			MaxHotShardBytes: 8 * 1024 * 1024,
			MinShardAgeHours: int(conf.CompactionMinAge.Hours()),
			Compression:      "zstd",
		},
		Summary: SummaryPolicy{
			AbstractWordsMin:      100,
			AbstractWordsMax:      200,
			OverviewWordsMax:      2000,
			RefreshIntervalMinute: int(conf.SummaryRefreshInterval / time.Minute),
		},
	}
}
