package repositories

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/SarthkJadhv/sift/internal/agents/codex"
	"github.com/SarthkJadhv/sift/internal/storage"
)

func newTestCodexRepository(
	t *testing.T,
) (*CodexRepository, context.Context, string) {
	t.Helper()

	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "test.db")

	db, err := storage.OpenSQLite(ctx, dbPath)
	if err != nil {
		t.Fatalf("OpenSQLite() failed: %v", err)
	}

	if err := db.Migrate(ctx); err != nil {
		db.Close()
		t.Fatalf("Migrate() failed: %v", err)
	}

	projectRepo, err := NewProjectRepository(db.DB)
	if err != nil {
		db.Close()
		t.Fatalf("NewProjectRepository() failed: %v", err)
	}

	project := Project{
		ID:        "project-1",
		Name:      "sift",
		RootPath:  filepath.Join(t.TempDir(), "sift"),
		CreatedAt: time.Now().UTC().Truncate(time.Second),
	}

	if err := projectRepo.Create(ctx, project); err != nil {
		db.Close()
		t.Fatalf("Create() failed: %v", err)
	}

	repo := NewCodexRepository(db.DB)

	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Errorf("failed to close database: %v", err)
		}
	})

	return repo, ctx, project.ID
}

func newTestCodexSession() *codex.Session {
	startedAt := time.Date(
		2026, 9, 24, 11, 46, 11, 0, time.UTC,
	)

	completedAt := startedAt.Add(2 * time.Second)

	return &codex.Session{
		ID:               "session-1",
		StartedAt:        startedAt,
		WorkingDirectory: "/workspace/sift",
		CLIVersion:       "0.156.1",
		ModelProvider:    "openai",
		Originator:       "codex-cli",
		ThreadSource:     "direct",

		Turns: []codex.Turn{
			{
				ID:                 "turn-1",
				Model:              "gpt-5",
				Effort:             "medium",
				StartedAt:          startedAt,
				CompletedAt:        completedAt,
				Duration:           2 * time.Second,
				TimeToFirstToken:   300 * time.Millisecond,
				ModelContextWindow: 128000,
			},
		},

		Responses: []codex.ResponseUsage{
			{
				ResponseID: "response-1",
				SessionID:  "session-1",
				ThreadID:   "thread-1",
				TurnID:     "turn-1",
				RootTurnID: "turn-1",
				Timestamp:  startedAt,
				Ordinal:    5,

				Usage: codex.TokenUsage{
					InputTokens:           1000,
					CachedInputTokens:     400,
					CacheWriteInputTokens: 0,
					OutputTokens:          200,
					ReasoningOutputTokens: 50,
					TotalTokens:           1200,
				},

				TurnCumulative: codex.TokenUsage{
					InputTokens:  1000,
					OutputTokens: 200,
					TotalTokens:  1200,
				},

				SessionCumulative: codex.TokenUsage{
					InputTokens:  1000,
					OutputTokens: 200,
					TotalTokens:  1200,
				},
			},
		},

		ToolActivities: []codex.ToolActivity{
			{
				CallID:        "call-1",
				ItemID:        "item-1",
				Name:          "exec",
				Status:        "completed",
				StartedAt:     startedAt,
				CompletedAt:   completedAt,
				Duration:      2 * time.Second,
				HasOutput:     true,
				CallOrdinal:   6,
				OutputOrdinal: 8,
			},
		},
	}
}

func TestCodexRepositoryImport(t *testing.T) {
	repo, ctx, projectID := newTestCodexRepository(t)

	session := newTestCodexSession()
	rolloutPath := filepath.Join(t.TempDir(), "rollout.jsonl")

	err := repo.Import(
		ctx,
		projectID,
		rolloutPath,
		1024,
		123456,
		session,
	)
	if err != nil {
		t.Fatalf("Import() failed: %v", err)
	}

	var sessionCount int
	var turnCount int
	var responseCount int
	var toolCount int

	queries := []struct {
		query string
		dest  *int
	}{
		{"SELECT COUNT(*) FROM codex_sessions", &sessionCount},
		{"SELECT COUNT(*) FROM codex_turns", &turnCount},
		{"SELECT COUNT(*) FROM codex_response_usage", &responseCount},
		{"SELECT COUNT(*) FROM codex_tool_activities", &toolCount},
	}

	for _, item := range queries {
		if err := repo.db.QueryRowContext(
			ctx,
			item.query,
		).Scan(item.dest); err != nil {
			t.Fatalf("query failed: %v", err)
		}
	}

	if sessionCount != 1 {
		t.Errorf("expected 1 session, got %d", sessionCount)
	}

	if turnCount != 1 {
		t.Errorf("expected 1 turn, got %d", turnCount)
	}

	if responseCount != 1 {
		t.Errorf("expected 1 response, got %d", responseCount)
	}

	if toolCount != 1 {
		t.Errorf("expected 1 tool activity, got %d", toolCount)
	}

	var gotProjectID string
	var gotCLIVersion string

	err = repo.db.QueryRowContext(
		ctx,
		`SELECT project_id, cli_version
		 FROM codex_sessions
		 WHERE id = ?`,
		session.ID,
	).Scan(&gotProjectID, &gotCLIVersion)

	if err != nil {
		t.Fatalf("reading session failed: %v", err)
	}

	if gotProjectID != projectID {
		t.Errorf("expected project ID %s, got %s",
			projectID, gotProjectID)
	}

	if gotCLIVersion != session.CLIVersion {
		t.Errorf("expected CLI version %s, got %s",
			session.CLIVersion, gotCLIVersion)
	}

	var inputTokens int64

	err = repo.db.QueryRowContext(
		ctx,
		`SELECT input_tokens
		 FROM codex_response_usage
		 WHERE session_id = ? AND response_id = ?`,
		session.ID,
		"response-1",
	).Scan(&inputTokens)

	if err != nil {
		t.Fatalf("reading token usage failed: %v", err)
	}

	if inputTokens != 1000 {
		t.Errorf("expected 1000 input tokens, got %d", inputTokens)
	}
}

func TestCodexRepositoryImportIsIdempotent(t *testing.T) {
	repo, ctx, projectID := newTestCodexRepository(t)

	session := newTestCodexSession()
	rolloutPath := filepath.Join(t.TempDir(), "rollout.jsonl")

	for i := 0; i < 2; i++ {
		if err := repo.Import(
			ctx,
			projectID,
			rolloutPath,
			1024,
			123456,
			session,
		); err != nil {
			t.Fatalf("Import() iteration %d failed: %v", i+1, err)
		}
	}

	tables := []string{
		"codex_sessions",
		"codex_turns",
		"codex_response_usage",
		"codex_tool_activities",
	}

	for _, table := range tables {
		var count int

		query := "SELECT COUNT(*) FROM " + table

		if err := repo.db.QueryRowContext(ctx, query).Scan(&count); err != nil {
			t.Fatalf("counting %s failed: %v", table, err)
		}

		if count != 1 {
			t.Errorf("expected 1 record in %s after re-import, got %d",
				table, count)
		}
	}
}

func TestCodexRepositoryImportRollsBackOnFailure(t *testing.T) {
	repo, ctx, projectID := newTestCodexRepository(t)

	session := newTestCodexSession()
	rolloutPath := filepath.Join(t.TempDir(), "rollout.jsonl")

	if err := repo.Import(
		ctx,
		projectID,
		rolloutPath,
		1024,
		123456,
		session,
	); err != nil {
		t.Fatalf("initial Import() failed: %v", err)
	}

	// Prepare an invalid re-import.
	// The empty turn ID violates the database CHECK constraint.
	invalidSession := *session
	invalidSession.CLIVersion = "0.999.0"
	invalidSession.Turns = []codex.Turn{
		{
			ID: "",
		},
	}

	err := repo.Import(
		ctx,
		projectID,
		rolloutPath,
		2048,
		654321,
		&invalidSession,
	)

	if err == nil {
		t.Fatal("expected Import() to fail for invalid turn")
	}

	// The original session metadata must remain unchanged.
	var cliVersion string

	err = repo.db.QueryRowContext(
		ctx,
		`SELECT cli_version
		 FROM codex_sessions
		 WHERE id = ?`,
		session.ID,
	).Scan(&cliVersion)

	if err != nil {
		t.Fatalf("reading session after rollback failed: %v", err)
	}

	if cliVersion != session.CLIVersion {
		t.Errorf("expected original CLI version %s, got %s",
			session.CLIVersion, cliVersion)
	}

	// Original child records must also remain intact.
	for _, table := range []string{
		"codex_turns",
		"codex_response_usage",
		"codex_tool_activities",
	} {
		var count int

		query := "SELECT COUNT(*) FROM " + table

		if err := repo.db.QueryRowContext(ctx, query).Scan(&count); err != nil {
			t.Fatalf("counting %s failed: %v", table, err)
		}

		if count != 1 {
			t.Errorf("expected original record in %s after rollback, got %d",
				table, count)
		}
	}
}
func TestCodexRepositoryGetEffortUsage(t *testing.T) {
	repo, ctx, projectID := newTestCodexRepository(t)

	session := newTestCodexSession()

	session.Turns = append(session.Turns, codex.Turn{
		ID:     "turn-2",
		Model:  "gpt-5.1",
		Effort: "high",
	})

	session.Responses = append(session.Responses, codex.ResponseUsage{
		ResponseID: "response-2",
		SessionID:  session.ID,
		ThreadID:   "thread-1",
		TurnID:     "turn-2",
		Timestamp:  session.StartedAt.Add(time.Minute),
		Usage: codex.TokenUsage{
			InputTokens:  2000,
			OutputTokens: 300,
			TotalTokens:  2300,
		},
	})

	err := repo.Import(
		ctx,
		projectID,
		filepath.Join(t.TempDir(), "rollout.jsonl"),
		2048,
		123456,
		session,
	)
	if err != nil {
		t.Fatalf("Import() failed: %v", err)
	}

	results, err := repo.GetEffortUsage(ctx, projectID)
	if err != nil {
		t.Fatalf("GetEffortUsage() failed: %v", err)
	}

	if len(results) != 2 {
		t.Fatalf("expected 2 effort groups, got %d", len(results))
	}

	expected := map[string]struct {
		responses int64
		input     int64
		total     int64
	}{
		"medium": {
			responses: 1,
			input:     1000,
			total:     1200,
		},
		"high": {
			responses: 1,
			input:     2000,
			total:     2300,
		},
	}

	for _, result := range results {
		want, exists := expected[result.Effort]
		if !exists {
			t.Errorf("unexpected effort: %s", result.Effort)
			continue
		}

		if result.Responses != want.responses {
			t.Errorf("%s: responses = %d, want %d",
				result.Effort, result.Responses, want.responses)
		}

		if result.InputTokens != want.input {
			t.Errorf("%s: input tokens = %d, want %d",
				result.Effort, result.InputTokens, want.input)
		}

		if result.TotalTokens != want.total {
			t.Errorf("%s: total tokens = %d, want %d",
				result.Effort, result.TotalTokens, want.total)
		}
	}
}

func TestCodexRepositoryListSessions(t *testing.T) {
	repo, ctx, projectID := newTestCodexRepository(t)

	first := newTestCodexSession()

	if err := repo.Import(
		ctx,
		projectID,
		filepath.Join(t.TempDir(), "first.jsonl"),
		1024,
		123456,
		first,
	); err != nil {
		t.Fatalf("first Import() failed: %v", err)
	}

	// Create a second session with a later start time.
	second := *newTestCodexSession()
	second.ID = "session-2"
	second.StartedAt = first.StartedAt.Add(time.Hour)

	second.Turns = append([]codex.Turn(nil), first.Turns...)
	second.Turns[0].ID = "turn-2"
	second.Turns[0].Model = "gpt-5.1"

	second.Responses = append([]codex.ResponseUsage(nil), first.Responses...)
	second.Responses[0].ResponseID = "response-2"
	second.Responses[0].SessionID = second.ID
	second.Responses[0].TurnID = "turn-2"

	second.ToolActivities = nil

	if err := repo.Import(
		ctx,
		projectID,
		filepath.Join(t.TempDir(), "second.jsonl"),
		2048,
		123457,
		&second,
	); err != nil {
		t.Fatalf("second Import() failed: %v", err)
	}

	results, err := repo.ListSessions(ctx, projectID, 10)
	if err != nil {
		t.Fatalf("ListSessions() failed: %v", err)
	}

	if len(results) != 2 {
		t.Fatalf("expected 2 sessions, got %d", len(results))
	}

	// Newest session should be first.
	if results[0].SessionID != "session-2" {
		t.Errorf("first session = %s, want session-2",
			results[0].SessionID)
	}

	if results[0].Models != "gpt-5.1" {
		t.Errorf("models = %q, want gpt-5.1", results[0].Models)
	}

	if results[0].TotalTokens != 1200 {
		t.Errorf("total tokens = %d, want 1200",
			results[0].TotalTokens)
	}

	if results[0].Turns != 1 {
		t.Errorf("turns = %d, want 1", results[0].Turns)
	}

	if results[0].TotalTurnDuration != 2*time.Second {
		t.Errorf("turn duration = %v, want 2s",
			results[0].TotalTurnDuration)
	}
}

func TestCodexRepositoryGetStats(t *testing.T) {
	repo, ctx, projectID := newTestCodexRepository(t)
	session := newTestCodexSession()

	err := repo.Import(ctx, projectID,
		filepath.Join(t.TempDir(), "rollout.jsonl"),
		1024, 123456, session)
	if err != nil {
		t.Fatalf("Import() failed: %v", err)
	}

	stats, err := repo.GetStats(ctx, projectID)
	if err != nil {
		t.Fatalf("GetStats() failed: %v", err)
	}

	if stats.Sessions != 1 ||
		stats.Turns != 1 ||
		stats.CompletedTurns != 1 ||
		stats.Responses != 1 ||
		stats.InputTokens != 1000 ||
		stats.CachedInputTokens != 400 ||
		stats.OutputTokens != 200 ||
		stats.ReasoningOutputTokens != 50 ||
		stats.TotalTokens != 1200 ||
		stats.ToolCalls != 1 ||
		stats.ToolsWithOutput != 1 {
		t.Fatalf("unexpected Codex stats: %+v", stats)
	}

	if stats.AverageTurnDuration != 2*time.Second {
		t.Errorf("AverageTurnDuration = %v, want 2s",
			stats.AverageTurnDuration)
	}
}
func TestCodexRepositoryGetModelUsage(t *testing.T) {
	repo, ctx, projectID := newTestCodexRepository(t)
	session := newTestCodexSession()

	session.Turns = append(session.Turns, codex.Turn{
		ID:     "turn-2",
		Model:  "gpt-5.1",
		Effort: "high",
	})

	session.Responses = append(session.Responses, codex.ResponseUsage{
		ResponseID: "response-2",
		SessionID:  session.ID,
		ThreadID:   "thread-1",
		TurnID:     "turn-2",
		Timestamp:  session.StartedAt.Add(time.Minute),
		Usage: codex.TokenUsage{
			InputTokens:  2000,
			OutputTokens: 300,
			TotalTokens:  2300,
		},
	})

	err := repo.Import(ctx, projectID,
		filepath.Join(t.TempDir(), "rollout.jsonl"),
		2048, 123456, session)
	if err != nil {
		t.Fatalf("Import() failed: %v", err)
	}

	results, err := repo.GetModelUsage(ctx, projectID)
	if err != nil {
		t.Fatalf("GetModelUsage() failed: %v", err)
	}

	expected := map[string]int64{
		"gpt-5":   1200,
		"gpt-5.1": 2300,
	}

	if len(results) != len(expected) {
		t.Fatalf("expected 2 model groups, got %d", len(results))
	}

	for _, result := range results {
		want, exists := expected[result.Model]
		if !exists {
			t.Errorf("unexpected model: %s", result.Model)
			continue
		}

		if result.TotalTokens != want {
			t.Errorf("%s: total tokens = %d, want %d",
				result.Model, result.TotalTokens, want)
		}
	}
}
