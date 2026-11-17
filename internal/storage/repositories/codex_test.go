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
func TestCodexRepositoryGetStats(t *testing.T) {
	repo, ctx, projectID := newTestCodexRepository(t)

	session := newTestCodexSession()

	err := repo.Import(
		ctx,
		projectID,
		filepath.Join(t.TempDir(), "rollout.jsonl"),
		1024,
		123456,
		session,
	)
	if err != nil {
		t.Fatalf("Import() failed: %v", err)
	}

	stats, err := repo.GetStats(ctx, projectID)
	if err != nil {
		t.Fatalf("GetStats() failed: %v", err)
	}

	tests := []struct {
		name string
		got  int64
		want int64
	}{
		{"Sessions", stats.Sessions, 1},
		{"Turns", stats.Turns, 1},
		{"CompletedTurns", stats.CompletedTurns, 1},
		{"Responses", stats.Responses, 1},
		{"InputTokens", stats.InputTokens, 1000},
		{"CachedInputTokens", stats.CachedInputTokens, 400},
		{"OutputTokens", stats.OutputTokens, 200},
		{"ReasoningOutputTokens", stats.ReasoningOutputTokens, 50},
		{"TotalTokens", stats.TotalTokens, 1200},
		{"ToolCalls", stats.ToolCalls, 1},
		{"ToolsWithOutput", stats.ToolsWithOutput, 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got != tt.want {
				t.Errorf("got %d, want %d", tt.got, tt.want)
			}
		})
	}

	if stats.AverageTurnDuration != 2*time.Second {
		t.Errorf(
			"AverageTurnDuration = %v, want 2s",
			stats.AverageTurnDuration,
		)
	}
}
