package codex

import "time"

// Session represents a Codex coding session.
type Session struct {
	ID               string
	StartedAt        time.Time
	WorkingDirectory string
	CLIVersion       string
	ModelProvider    string
	Originator       string
	ThreadSource     string

	Turns            []Turn
	Responses        []ResponseUsage
	ToolActivities   []ToolActivity
	ContextSnapshots []ContextSnapshot
}

// Turn contains the model configuration observed for a Codex turn.
type Turn struct {
	ID                 string
	Model              string
	Effort             string
	StartedAt          time.Time
	CompletedAt        time.Time
	Duration           time.Duration
	TimeToFirstToken   time.Duration
	ModelContextWindow int64
}

// TokenUsage represents token usage reported by Codex.
type TokenUsage struct {
	InputTokens           int64 `json:"input_tokens"`
	CachedInputTokens     int64 `json:"cached_input_tokens"`
	CacheWriteInputTokens int64 `json:"cache_write_input_tokens"`
	OutputTokens          int64 `json:"output_tokens"`
	ReasoningOutputTokens int64 `json:"reasoning_output_tokens"`
	TotalTokens           int64 `json:"total_tokens"`
}

// ResponseUsage represents token usage for one model response.
type ResponseUsage struct {
	ResponseID string
	SessionID  string
	ThreadID   string
	TurnID     string
	RootTurnID string

	Timestamp time.Time
	Ordinal   int64

	Usage             TokenUsage
	TurnCumulative    TokenUsage
	SessionCumulative TokenUsage
}

// ToolActivity represents metadata about one Codex tool invocation.
type ToolActivity struct {
	CallID string
	ItemID string

	Name   string
	Status string

	StartedAt   time.Time
	CompletedAt time.Time
	Duration    time.Duration

	HasOutput bool

	CallOrdinal   int64
	OutputOrdinal int64
}

// ContextSnapshot represents a point-in-time context window measurement.
type ContextSnapshot struct {
	ObservedAt              time.Time
	Sequence                int64
	ActiveContextTokens     int64
	ContextWindowTokens     int64
	CumulativeSessionTokens int64
}
