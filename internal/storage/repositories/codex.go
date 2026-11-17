package repositories

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/SarthkJadhv/sift/internal/agents/codex"
)

// CodexRepository persists parsed Codex session activity.
type CodexRepository struct {
	db *sql.DB
}

// CodexStats contains aggregated activity for a SIFT project.
type CodexStats struct {
	Sessions              int64
	Turns                 int64
	CompletedTurns        int64
	Responses             int64
	InputTokens           int64
	CachedInputTokens     int64
	OutputTokens          int64
	ReasoningOutputTokens int64
	TotalTokens           int64
	ToolCalls             int64
	ToolsWithOutput       int64
	AverageTurnDuration   time.Duration
}

func NewCodexRepository(db *sql.DB) *CodexRepository {
	return &CodexRepository{db: db}
}

// IsUnchanged checks whether a rollout file has already been imported
// with the same filesystem metadata for the specified project.
func (r *CodexRepository) IsUnchanged(
	ctx context.Context,
	projectID string,
	rolloutPath string,
	fileSizeBytes int64,
	fileModTimeNS int64,
) (bool, error) {
	if r == nil || r.db == nil {
		return false, errors.New("codex repository database is nil")
	}

	if strings.TrimSpace(projectID) == "" {
		return false, errors.New("project ID cannot be empty")
	}

	if strings.TrimSpace(rolloutPath) == "" {
		return false, errors.New("rollout path cannot be empty")
	}

	if fileSizeBytes < 0 || fileModTimeNS < 0 {
		return false, errors.New("file metadata cannot be negative")
	}

	var unchanged bool

	err := r.db.QueryRowContext(
		ctx,
		`SELECT EXISTS (
			SELECT 1
			FROM codex_sessions
			WHERE project_id = ?
			  AND rollout_path = ?
			  AND file_size_bytes = ?
			  AND file_mod_time_ns = ?
		)`,
		projectID,
		rolloutPath,
		fileSizeBytes,
		fileModTimeNS,
	).Scan(&unchanged)

	if err != nil {
		return false, fmt.Errorf("check rollout import status: %w", err)
	}

	return unchanged, nil
}

// GetStats returns aggregated Codex activity for a SIFT project.
func (r *CodexRepository) GetStats(
	ctx context.Context,
	projectID string,
) (CodexStats, error) {
	if r == nil || r.db == nil {
		return CodexStats{}, errors.New("codex repository database is nil")
	}

	if strings.TrimSpace(projectID) == "" {
		return CodexStats{}, errors.New("project ID cannot be empty")
	}

	const query = `
		SELECT
			(SELECT COUNT(*)
			 FROM codex_sessions
			 WHERE project_id = ?),

			(SELECT COUNT(*)
			 FROM codex_turns t
			 JOIN codex_sessions s ON s.id = t.session_id
			 WHERE s.project_id = ?),

			(SELECT COUNT(*)
			 FROM codex_turns t
			 JOIN codex_sessions s ON s.id = t.session_id
			 WHERE s.project_id = ?
			   AND t.completed_at <> ''),

			(SELECT COUNT(*)
			 FROM codex_response_usage u
			 JOIN codex_sessions s ON s.id = u.session_id
			 WHERE s.project_id = ?),

			(SELECT COALESCE(SUM(u.input_tokens), 0)
			 FROM codex_response_usage u
			 JOIN codex_sessions s ON s.id = u.session_id
			 WHERE s.project_id = ?),

			(SELECT COALESCE(SUM(u.cached_input_tokens), 0)
			 FROM codex_response_usage u
			 JOIN codex_sessions s ON s.id = u.session_id
			 WHERE s.project_id = ?),

			(SELECT COALESCE(SUM(u.output_tokens), 0)
			 FROM codex_response_usage u
			 JOIN codex_sessions s ON s.id = u.session_id
			 WHERE s.project_id = ?),

			(SELECT COALESCE(SUM(u.reasoning_output_tokens), 0)
			 FROM codex_response_usage u
			 JOIN codex_sessions s ON s.id = u.session_id
			 WHERE s.project_id = ?),

			(SELECT COALESCE(SUM(u.total_tokens), 0)
			 FROM codex_response_usage u
			 JOIN codex_sessions s ON s.id = u.session_id
			 WHERE s.project_id = ?),

			(SELECT COUNT(*)
			 FROM codex_tool_activities t
			 JOIN codex_sessions s ON s.id = t.session_id
			 WHERE s.project_id = ?),

			(SELECT COUNT(*)
			 FROM codex_tool_activities t
			 JOIN codex_sessions s ON s.id = t.session_id
			 WHERE s.project_id = ?
			   AND t.has_output = 1),

			(SELECT COALESCE(AVG(t.duration_ns), 0)
			 FROM codex_turns t
			 JOIN codex_sessions s ON s.id = t.session_id
			 WHERE s.project_id = ?
			   AND t.completed_at <> '')
	`

	var stats CodexStats
	var averageDurationNS float64

	err := r.db.QueryRowContext(
		ctx,
		query,
		projectID,
		projectID,
		projectID,
		projectID,
		projectID,
		projectID,
		projectID,
		projectID,
		projectID,
		projectID,
		projectID,
		projectID,
	).Scan(
		&stats.Sessions,
		&stats.Turns,
		&stats.CompletedTurns,
		&stats.Responses,
		&stats.InputTokens,
		&stats.CachedInputTokens,
		&stats.OutputTokens,
		&stats.ReasoningOutputTokens,
		&stats.TotalTokens,
		&stats.ToolCalls,
		&stats.ToolsWithOutput,
		&averageDurationNS,
	)
	if err != nil {
		return CodexStats{}, fmt.Errorf("query Codex statistics: %w", err)
	}

	stats.AverageTurnDuration = time.Duration(averageDurationNS)

	return stats, nil
}

// Import persists a complete Codex rollout and its activity.
// Re-importing a session replaces its child records transactionally.
func (r *CodexRepository) Import(
	ctx context.Context,
	projectID string,
	rolloutPath string,
	fileSizeBytes int64,
	fileModTimeNS int64,
	session *codex.Session,
) error {
	if r == nil || r.db == nil {
		return errors.New("codex repository database is nil")
	}

	if session == nil {
		return errors.New("codex session cannot be nil")
	}

	if strings.TrimSpace(session.ID) == "" {
		return errors.New("codex session ID cannot be empty")
	}

	if strings.TrimSpace(projectID) == "" {
		return errors.New("project ID cannot be empty")
	}

	if strings.TrimSpace(rolloutPath) == "" {
		return errors.New("rollout path cannot be empty")
	}

	if fileSizeBytes < 0 || fileModTimeNS < 0 {
		return errors.New("file metadata cannot be negative")
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin codex import transaction: %w", err)
	}
	defer tx.Rollback()

	if err := upsertCodexSession(
		ctx,
		tx,
		projectID,
		rolloutPath,
		fileSizeBytes,
		fileModTimeNS,
		session,
	); err != nil {
		return err
	}

	if err := deleteCodexSessionActivity(ctx, tx, session.ID); err != nil {
		return err
	}

	if err := insertCodexTurns(ctx, tx, session.ID, session.Turns); err != nil {
		return err
	}

	if err := insertCodexResponseUsage(
		ctx,
		tx,
		session.ID,
		session.Responses,
	); err != nil {
		return err
	}

	if err := insertCodexToolActivities(
		ctx,
		tx,
		session.ID,
		session.ToolActivities,
	); err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit codex import transaction: %w", err)
	}

	return nil
}

func upsertCodexSession(
	ctx context.Context,
	tx *sql.Tx,
	projectID string,
	rolloutPath string,
	fileSizeBytes int64,
	fileModTimeNS int64,
	session *codex.Session,
) error {
	query := `
		INSERT INTO codex_sessions (
			id,
			project_id,
			rollout_path,
			started_at,
			working_directory,
			cli_version,
			model_provider,
			originator,
			thread_source,
			file_size_bytes,
			file_mod_time_ns,
			last_imported_at
		)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			project_id = excluded.project_id,
			rollout_path = excluded.rollout_path,
			started_at = excluded.started_at,
			working_directory = excluded.working_directory,
			cli_version = excluded.cli_version,
			model_provider = excluded.model_provider,
			originator = excluded.originator,
			thread_source = excluded.thread_source,
			file_size_bytes = excluded.file_size_bytes,
			file_mod_time_ns = excluded.file_mod_time_ns,
			last_imported_at = excluded.last_imported_at
	`

	_, err := tx.ExecContext(
		ctx,
		query,
		session.ID,
		projectID,
		rolloutPath,
		formatCodexTimestamp(session.StartedAt),
		session.WorkingDirectory,
		session.CLIVersion,
		session.ModelProvider,
		session.Originator,
		session.ThreadSource,
		fileSizeBytes,
		fileModTimeNS,
		time.Now().UTC().Format(time.RFC3339Nano),
	)
	if err != nil {
		return fmt.Errorf("upsert codex session: %w", err)
	}

	return nil
}

func deleteCodexSessionActivity(
	ctx context.Context,
	tx *sql.Tx,
	sessionID string,
) error {
	tables := []string{
		"codex_turns",
		"codex_response_usage",
		"codex_tool_activities",
	}

	for _, table := range tables {
		query := "DELETE FROM " + table + " WHERE session_id = ?"

		if _, err := tx.ExecContext(ctx, query, sessionID); err != nil {
			return fmt.Errorf("delete previous records from %s: %w", table, err)
		}
	}

	return nil
}

func insertCodexTurns(
	ctx context.Context,
	tx *sql.Tx,
	sessionID string,
	turns []codex.Turn,
) error {
	query := `
		INSERT INTO codex_turns (
			session_id,
			turn_id,
			model,
			effort,
			started_at,
			completed_at,
			duration_ns,
			time_to_first_token_ns,
			model_context_window
		)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`

	for _, turn := range turns {
		_, err := tx.ExecContext(
			ctx,
			query,
			sessionID,
			turn.ID,
			turn.Model,
			turn.Effort,
			formatCodexTimestamp(turn.StartedAt),
			formatCodexTimestamp(turn.CompletedAt),
			int64(turn.Duration),
			int64(turn.TimeToFirstToken),
			turn.ModelContextWindow,
		)
		if err != nil {
			return fmt.Errorf("insert codex turn %s: %w", turn.ID, err)
		}
	}

	return nil
}

func insertCodexResponseUsage(
	ctx context.Context,
	tx *sql.Tx,
	sessionID string,
	responses []codex.ResponseUsage,
) error {
	query := `
		INSERT INTO codex_response_usage (
			session_id,
			response_id,
			thread_id,
			turn_id,
			root_turn_id,
			timestamp,
			ordinal,

			input_tokens,
			cached_input_tokens,
			cache_write_input_tokens,
			output_tokens,
			reasoning_output_tokens,
			total_tokens,

			turn_input_tokens,
			turn_cached_input_tokens,
			turn_cache_write_input_tokens,
			turn_output_tokens,
			turn_reasoning_output_tokens,
			turn_total_tokens,

			session_input_tokens,
			session_cached_input_tokens,
			session_cache_write_input_tokens,
			session_output_tokens,
			session_reasoning_output_tokens,
			session_total_tokens
		)
		VALUES (
			?, ?, ?, ?, ?, ?, ?,
			?, ?, ?, ?, ?, ?,
			?, ?, ?, ?, ?, ?,
			?, ?, ?, ?, ?, ?
		)
	`

	for _, response := range responses {
		_, err := tx.ExecContext(
			ctx,
			query,
			sessionID,
			response.ResponseID,
			response.ThreadID,
			response.TurnID,
			response.RootTurnID,
			formatCodexTimestamp(response.Timestamp),
			response.Ordinal,

			response.Usage.InputTokens,
			response.Usage.CachedInputTokens,
			response.Usage.CacheWriteInputTokens,
			response.Usage.OutputTokens,
			response.Usage.ReasoningOutputTokens,
			response.Usage.TotalTokens,

			response.TurnCumulative.InputTokens,
			response.TurnCumulative.CachedInputTokens,
			response.TurnCumulative.CacheWriteInputTokens,
			response.TurnCumulative.OutputTokens,
			response.TurnCumulative.ReasoningOutputTokens,
			response.TurnCumulative.TotalTokens,

			response.SessionCumulative.InputTokens,
			response.SessionCumulative.CachedInputTokens,
			response.SessionCumulative.CacheWriteInputTokens,
			response.SessionCumulative.OutputTokens,
			response.SessionCumulative.ReasoningOutputTokens,
			response.SessionCumulative.TotalTokens,
		)
		if err != nil {
			return fmt.Errorf(
				"insert codex response usage %s: %w",
				response.ResponseID,
				err,
			)
		}
	}

	return nil
}

func insertCodexToolActivities(
	ctx context.Context,
	tx *sql.Tx,
	sessionID string,
	activities []codex.ToolActivity,
) error {
	query := `
		INSERT INTO codex_tool_activities (
			session_id,
			call_id,
			item_id,
			name,
			status,
			started_at,
			completed_at,
			duration_ns,
			has_output,
			call_ordinal,
			output_ordinal
		)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`

	for _, activity := range activities {
		_, err := tx.ExecContext(
			ctx,
			query,
			sessionID,
			activity.CallID,
			activity.ItemID,
			activity.Name,
			activity.Status,
			formatCodexTimestamp(activity.StartedAt),
			formatCodexTimestamp(activity.CompletedAt),
			int64(activity.Duration),
			activity.HasOutput,
			activity.CallOrdinal,
			activity.OutputOrdinal,
		)
		if err != nil {
			return fmt.Errorf(
				"insert codex tool activity %s: %w",
				activity.CallID,
				err,
			)
		}
	}

	return nil
}

func formatCodexTimestamp(value time.Time) string {
	if value.IsZero() {
		return ""
	}

	return value.UTC().Format(time.RFC3339Nano)
}
