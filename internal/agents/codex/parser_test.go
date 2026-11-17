package codex

import (
	"os"
	"strings"
	"testing"
	"time"
)

func TestParseRolloutSessionMetadata(t *testing.T) {
	input := `{"timestamp":"2026-09-24T11:46:11Z","ordinal":0,"type":"session_meta","payload":{"id":"session-123","session_id":"session-123","timestamp":"2026-09-24T11:46:11Z","cwd":"/workspace/project","cli_version":"0.156.1","model_provider":"openai","originator":"codex-cli","source":{"subagent":{}},"thread_source":"direct"}}`

	session, err := ParseRollout(strings.NewReader(input))
	if err != nil {
		t.Fatalf("ParseRollout() error = %v", err)
	}

	if session.ID != "session-123" {
		t.Errorf("ID = %q, want %q", session.ID, "session-123")
	}

	if session.WorkingDirectory != "/workspace/project" {
		t.Errorf("WorkingDirectory = %q, want %q",
			session.WorkingDirectory, "/workspace/project")
	}

	if session.CLIVersion != "0.156.1" {
		t.Errorf("CLIVersion = %q, want %q",
			session.CLIVersion, "0.156.1")
	}

	if session.ModelProvider != "openai" {
		t.Errorf("ModelProvider = %q, want %q",
			session.ModelProvider, "openai")
	}

	expectedTime := time.Date(
		2026, 9, 24, 11, 46, 11, 0, time.UTC,
	)

	if !session.StartedAt.Equal(expectedTime) {
		t.Errorf("StartedAt = %v, want %v",
			session.StartedAt, expectedTime)
	}
}

func TestParseRolloutTokenUsage(t *testing.T) {
	input := `{"timestamp":"2026-09-24T11:46:15Z","ordinal":5,"type":"token_usage_record","payload":{"response_id":"response-123","session_id":"session-123","thread_id":"thread-123","turn_id":"turn-123","root_turn_id":"turn-123","usage":{"input_tokens":1000,"cached_input_tokens":400,"cache_write_input_tokens":0,"output_tokens":200,"reasoning_output_tokens":50,"total_tokens":1200},"turn_token_usage":{"input_tokens":1000,"cached_input_tokens":400,"cache_write_input_tokens":0,"output_tokens":200,"reasoning_output_tokens":50,"total_tokens":1200},"thread_token_usage":{"input_tokens":1000,"cached_input_tokens":400,"cache_write_input_tokens":0,"output_tokens":200,"reasoning_output_tokens":50,"total_tokens":1200}}}`

	session, err := ParseRollout(strings.NewReader(input))
	if err != nil {
		t.Fatalf("ParseRollout() error = %v", err)
	}

	if len(session.Responses) != 1 {
		t.Fatalf("Responses length = %d, want 1", len(session.Responses))
	}

	response := session.Responses[0]

	if response.ResponseID != "response-123" {
		t.Errorf("ResponseID = %q, want %q",
			response.ResponseID, "response-123")
	}

	if response.Ordinal != 5 {
		t.Errorf("Ordinal = %d, want 5", response.Ordinal)
	}

	if response.Usage.InputTokens != 1000 {
		t.Errorf("InputTokens = %d, want 1000",
			response.Usage.InputTokens)
	}

	if response.Usage.CachedInputTokens != 400 {
		t.Errorf("CachedInputTokens = %d, want 400",
			response.Usage.CachedInputTokens)
	}

	if response.Usage.OutputTokens != 200 {
		t.Errorf("OutputTokens = %d, want 200",
			response.Usage.OutputTokens)
	}

	if response.Usage.ReasoningOutputTokens != 50 {
		t.Errorf("ReasoningOutputTokens = %d, want 50",
			response.Usage.ReasoningOutputTokens)
	}

	if response.SessionCumulative.TotalTokens != 1200 {
		t.Errorf("SessionCumulative.TotalTokens = %d, want 1200",
			response.SessionCumulative.TotalTokens)
	}
}

func TestParseRolloutMultipleTurns(t *testing.T) {
	input := `{"type":"turn_context","payload":{"turn_id":"turn-1","model":"gpt-5","effort":"medium"}}
{"type":"turn_context","payload":{"turn_id":"turn-2","model":"gpt-5-codex","effort":"high"}}`

	session, err := ParseRollout(strings.NewReader(input))
	if err != nil {
		t.Fatalf("ParseRollout() error = %v", err)
	}

	if len(session.Turns) != 2 {
		t.Fatalf("Turns length = %d, want 2", len(session.Turns))
	}

	if session.Turns[0].ID != "turn-1" {
		t.Errorf("First turn ID = %q, want turn-1", session.Turns[0].ID)
	}

	if session.Turns[0].Model != "gpt-5" {
		t.Errorf("First turn model = %q, want gpt-5",
			session.Turns[0].Model)
	}

	if session.Turns[1].ID != "turn-2" {
		t.Errorf("Second turn ID = %q, want turn-2", session.Turns[1].ID)
	}

	if session.Turns[1].Effort != "high" {
		t.Errorf("Second turn effort = %q, want high",
			session.Turns[1].Effort)
	}
}

func TestParseRolloutUpdatesExistingTurn(t *testing.T) {
	input := `{"type":"turn_context","payload":{"turn_id":"turn-1","model":"gpt-5","effort":"medium"}}
{"type":"turn_context","payload":{"turn_id":"turn-1","model":"gpt-5.1","effort":"high"}}`

	session, err := ParseRollout(strings.NewReader(input))
	if err != nil {
		t.Fatalf("ParseRollout() error = %v", err)
	}

	if len(session.Turns) != 1 {
		t.Fatalf("Turns length = %d, want 1", len(session.Turns))
	}

	if session.Turns[0].Model != "gpt-5.1" {
		t.Errorf("Model = %q, want gpt-5.1", session.Turns[0].Model)
	}

	if session.Turns[0].Effort != "high" {
		t.Errorf("Effort = %q, want high", session.Turns[0].Effort)
	}
}

func TestParseRolloutIgnoresUnknownRecordTypes(t *testing.T) {
	input := `{"type":"unknown_record","payload":{"some_field":"ignored"}}
{"type":"turn_context","payload":{"turn_id":"turn-1","model":"gpt-5","effort":"medium"}}`

	session, err := ParseRollout(strings.NewReader(input))
	if err != nil {
		t.Fatalf("ParseRollout() error = %v", err)
	}

	if len(session.Turns) != 1 {
		t.Errorf("Turns length = %d, want 1", len(session.Turns))
	}
}

func TestParseRolloutRejectsMalformedJSON(t *testing.T) {
	input := `{"type":"session_meta","payload":`

	_, err := ParseRollout(strings.NewReader(input))
	if err == nil {
		t.Fatal("ParseRollout() error = nil, want an error")
	}
}

func TestParseRolloutRejectsInvalidTimestamp(t *testing.T) {
	input := `{"timestamp":"not-a-timestamp","type":"token_usage_record","payload":{}}`

	_, err := ParseRollout(strings.NewReader(input))
	if err == nil {
		t.Fatal("ParseRollout() error = nil, want an error")
	}
}

func TestParseRealRollout(t *testing.T) {
	path := os.Getenv("SIFT_TEST_ROLLOUT")
	if path == "" {
		t.Skip("SIFT_TEST_ROLLOUT not set; skipping integration test")
	}

	session, err := ParseRolloutFile(path)
	if err != nil {
		t.Fatalf("ParseRolloutFile() error = %v", err)
	}

	if session.ID == "" {
		t.Error("session ID is empty")
	}

	if session.StartedAt.IsZero() {
		t.Error("session start time is missing")
	}

	if session.WorkingDirectory == "" {
		t.Error("session working directory is missing")
	}

	if len(session.Responses) == 0 {
		t.Fatal("no response usage records were parsed")
	}

	lastResponse := session.Responses[len(session.Responses)-1]

	var completedTurns int
	for _, turn := range session.Turns {
		if !turn.CompletedAt.IsZero() {
			completedTurns++
		}
	}

	var toolsWithOutput int
	var toolsWithDuration int

	for _, tool := range session.ToolActivities {
		if tool.HasOutput {
			toolsWithOutput++
		}
		if tool.Duration > 0 {
			toolsWithDuration++
		}
	}

	t.Logf("Session ID: %s", session.ID)
	t.Logf("Turns: %d (completed: %d)", len(session.Turns), completedTurns)
	t.Logf("Responses: %d", len(session.Responses))
	t.Logf("Session tokens: %d", lastResponse.SessionCumulative.TotalTokens)
	t.Logf("Tool activities: %d", len(session.ToolActivities))
	t.Logf("Tools with output: %d", toolsWithOutput)
	t.Logf("Tools with duration: %d", toolsWithDuration)
}
