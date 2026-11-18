package codex

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"time"
)

type rolloutRecord struct {
	Timestamp string          `json:"timestamp"`
	Ordinal   int64           `json:"ordinal"`
	Type      string          `json:"type"`
	Payload   json.RawMessage `json:"payload"`
	Sequence  int64           `json:"-"`
}

type sessionMetaPayload struct {
	ID            string `json:"id"`
	SessionID     string `json:"session_id"`
	Timestamp     string `json:"timestamp"`
	CWD           string `json:"cwd"`
	CLIVersion    string `json:"cli_version"`
	ModelProvider string `json:"model_provider"`
	Originator    string `json:"originator"`
	ThreadSource  string `json:"thread_source"`
}

type turnContextPayload struct {
	TurnID string `json:"turn_id"`
	Model  string `json:"model"`
	Effort string `json:"effort"`
}

type tokenUsagePayload struct {
	ResponseID       string     `json:"response_id"`
	SessionID        string     `json:"session_id"`
	ThreadID         string     `json:"thread_id"`
	TurnID           string     `json:"turn_id"`
	RootTurnID       string     `json:"root_turn_id"`
	Usage            TokenUsage `json:"usage"`
	TurnTokenUsage   TokenUsage `json:"turn_token_usage"`
	ThreadTokenUsage TokenUsage `json:"thread_token_usage"`
}

type taskEventPayload struct {
	Type               string `json:"type"`
	TurnID             string `json:"turn_id"`
	StartedAt          int64  `json:"started_at"`
	CompletedAt        int64  `json:"completed_at"`
	DurationMS         int64  `json:"duration_ms"`
	TimeToFirstTokenMS int64  `json:"time_to_first_token_ms"`
	ModelContextWindow int64  `json:"model_context_window"`
}

type tokenCountEventPayload struct {
	Type string `json:"type"`
	Info struct {
		TotalTokenUsage    TokenUsage `json:"total_token_usage"`
		LastTokenUsage     TokenUsage `json:"last_token_usage"`
		ModelContextWindow int64      `json:"model_context_window"`
	} `json:"info"`
}

type toolCallPayload struct {
	ID     string `json:"id"`
	CallID string `json:"call_id"`
	Name   string `json:"name"`
	Status string `json:"status"`
}

type toolOutputPayload struct {
	CallID string `json:"call_id"`
}

// ParseRollout reads a Codex JSONL rollout stream.
func ParseRollout(reader io.Reader) (*Session, error) {
	if reader == nil {
		return nil, errors.New("rollout reader cannot be nil")
	}

	decoder := json.NewDecoder(reader)

	session := &Session{
		Turns:            make([]Turn, 0),
		Responses:        make([]ResponseUsage, 0),
		ToolActivities:   make([]ToolActivity, 0),
		ContextSnapshots: make([]ContextSnapshot, 0),
	}

	turnIndexes := make(map[string]int)
	toolIndexes := make(map[string]int)

	recordNumber := 0

	for {
		var record rolloutRecord

		err := decoder.Decode(&record)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf(
				"decode rollout record %d: %w",
				recordNumber+1,
				err,
			)
		}

		recordNumber++
		record.Sequence = int64(recordNumber)

		switch record.Type {
		case "session_meta":
			if err := parseSessionMeta(record.Payload, session); err != nil {
				return nil, fmt.Errorf(
					"parse session metadata at record %d: %w",
					recordNumber,
					err,
				)
			}

		case "turn_context":
			if err := parseTurnContext(
				record.Payload,
				session,
				turnIndexes,
			); err != nil {
				return nil, fmt.Errorf(
					"parse turn context at record %d: %w",
					recordNumber,
					err,
				)
			}

		case "token_usage_record":
			response, err := parseTokenUsage(record)
			if err != nil {
				return nil, fmt.Errorf(
					"parse token usage at record %d: %w",
					recordNumber,
					err,
				)
			}

			session.Responses = append(session.Responses, response)

			if session.ID == "" {
				session.ID = response.SessionID
			}

		case "event_msg":
			if err := parseEventMessage(
				record,
				session,
				turnIndexes,
			); err != nil {
				return nil, fmt.Errorf(
					"parse event message at record %d: %w",
					recordNumber,
					err,
				)
			}

		case "response_item":
			if err := parseResponseItem(
				record,
				session,
				toolIndexes,
			); err != nil {
				return nil, fmt.Errorf(
					"parse response item at record %d: %w",
					recordNumber,
					err,
				)
			}
		}
	}

	return session, nil
}

// ParseRolloutFile opens and parses a Codex rollout file.
func ParseRolloutFile(path string) (*Session, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open rollout file: %w", err)
	}
	defer file.Close()

	return ParseRollout(file)
}

func parseSessionMeta(
	raw json.RawMessage,
	session *Session,
) error {
	var payload sessionMetaPayload

	if err := json.Unmarshal(raw, &payload); err != nil {
		return err
	}

	session.ID = payload.SessionID
	if session.ID == "" {
		session.ID = payload.ID
	}

	session.WorkingDirectory = payload.CWD
	session.CLIVersion = payload.CLIVersion
	session.ModelProvider = payload.ModelProvider
	session.Originator = payload.Originator
	session.ThreadSource = payload.ThreadSource

	startedAt, err := parseTimestamp(payload.Timestamp)
	if err != nil {
		return fmt.Errorf("invalid session timestamp: %w", err)
	}

	session.StartedAt = startedAt

	return nil
}

func parseTurnContext(
	raw json.RawMessage,
	session *Session,
	indexes map[string]int,
) error {
	var payload turnContextPayload

	if err := json.Unmarshal(raw, &payload); err != nil {
		return err
	}

	if payload.TurnID == "" {
		return nil
	}

	index := getOrCreateTurn(session, indexes, payload.TurnID)
	turn := &session.Turns[index]

	if payload.Model != "" {
		turn.Model = payload.Model
	}

	if payload.Effort != "" {
		turn.Effort = payload.Effort
	}

	return nil
}

func getOrCreateTurn(
	session *Session,
	indexes map[string]int,
	turnID string,
) int {
	if index, exists := indexes[turnID]; exists {
		return index
	}

	index := len(session.Turns)

	session.Turns = append(session.Turns, Turn{
		ID: turnID,
	})

	indexes[turnID] = index

	return index
}

func parseTokenUsage(record rolloutRecord) (ResponseUsage, error) {
	var payload tokenUsagePayload

	if err := json.Unmarshal(record.Payload, &payload); err != nil {
		return ResponseUsage{}, err
	}

	timestamp, err := parseTimestamp(record.Timestamp)
	if err != nil {
		return ResponseUsage{}, fmt.Errorf("invalid response timestamp: %w", err)
	}

	return ResponseUsage{
		ResponseID: payload.ResponseID,
		SessionID:  payload.SessionID,
		ThreadID:   payload.ThreadID,
		TurnID:     payload.TurnID,
		RootTurnID: payload.RootTurnID,

		Timestamp: timestamp,
		Ordinal:   record.Ordinal,

		Usage:             payload.Usage,
		TurnCumulative:    payload.TurnTokenUsage,
		SessionCumulative: payload.ThreadTokenUsage,
	}, nil
}

func parseEventMessage(
	record rolloutRecord,
	session *Session,
	turnIndexes map[string]int,
) error {
	var event struct {
		Type string `json:"type"`
	}

	if err := json.Unmarshal(record.Payload, &event); err != nil {
		return err
	}

	switch event.Type {
	case "token_count":
		var payload tokenCountEventPayload

		if err := json.Unmarshal(record.Payload, &payload); err != nil {
			return err
		}

		timestamp, err := parseTimestamp(record.Timestamp)
		if err != nil {
			return fmt.Errorf("invalid context snapshot timestamp: %w", err)
		}

		snapshot := ContextSnapshot{
			ObservedAt:              timestamp,
			Sequence:                record.Sequence,
			ActiveContextTokens:     payload.Info.LastTokenUsage.TotalTokens,
			ContextWindowTokens:     payload.Info.ModelContextWindow,
			CumulativeSessionTokens: payload.Info.TotalTokenUsage.TotalTokens,
		}

		session.ContextSnapshots = append(
			session.ContextSnapshots,
			snapshot,
		)

		return nil

	case "task_started", "task_complete":
		var payload taskEventPayload

		if err := json.Unmarshal(record.Payload, &payload); err != nil {
			return err
		}

		if payload.TurnID == "" {
			return nil
		}

		index := getOrCreateTurn(session, turnIndexes, payload.TurnID)
		turn := &session.Turns[index]

		if payload.ModelContextWindow > 0 {
			turn.ModelContextWindow = payload.ModelContextWindow
		}

		switch payload.Type {
		case "task_started":
			if payload.StartedAt > 0 {
				turn.StartedAt = time.UnixMilli(payload.StartedAt)
			} else if turn.StartedAt.IsZero() {
				timestamp, err := parseTimestamp(record.Timestamp)
				if err != nil {
					return err
				}
				turn.StartedAt = timestamp
			}

		case "task_complete":
			if turn.StartedAt.IsZero() && payload.StartedAt > 0 {
				turn.StartedAt = time.UnixMilli(payload.StartedAt)
			}

			if payload.CompletedAt > 0 {
				turn.CompletedAt = time.UnixMilli(payload.CompletedAt)
			} else {
				timestamp, err := parseTimestamp(record.Timestamp)
				if err != nil {
					return err
				}
				turn.CompletedAt = timestamp
			}

			if payload.DurationMS > 0 {
				turn.Duration = time.Duration(payload.DurationMS) * time.Millisecond
			} else if !turn.StartedAt.IsZero() && !turn.CompletedAt.IsZero() {
				turn.Duration = turn.CompletedAt.Sub(turn.StartedAt)
			}

			if payload.TimeToFirstTokenMS > 0 {
				turn.TimeToFirstToken =
					time.Duration(payload.TimeToFirstTokenMS) * time.Millisecond
			}
		}

		return nil

	default:
		return nil
	}
}

func parseResponseItem(
	record rolloutRecord,
	session *Session,
	toolIndexes map[string]int,
) error {
	var envelope struct {
		Type string `json:"type"`
	}

	if err := json.Unmarshal(record.Payload, &envelope); err != nil {
		return err
	}

	switch envelope.Type {
	case "custom_tool_call":
		var payload toolCallPayload

		if err := json.Unmarshal(record.Payload, &payload); err != nil {
			return err
		}

		if payload.CallID == "" {
			return nil
		}

		index := getOrCreateToolActivity(
			session,
			toolIndexes,
			payload.CallID,
		)

		activity := &session.ToolActivities[index]

		activity.ItemID = payload.ID
		activity.Name = payload.Name
		activity.Status = payload.Status
		activity.CallOrdinal = record.Ordinal

		timestamp, err := parseTimestamp(record.Timestamp)
		if err != nil {
			return err
		}

		activity.StartedAt = timestamp
		updateToolDuration(activity)

	case "custom_tool_call_output":
		var payload toolOutputPayload

		if err := json.Unmarshal(record.Payload, &payload); err != nil {
			return err
		}

		if payload.CallID == "" {
			return nil
		}

		index := getOrCreateToolActivity(
			session,
			toolIndexes,
			payload.CallID,
		)

		activity := &session.ToolActivities[index]

		activity.HasOutput = true
		activity.OutputOrdinal = record.Ordinal

		timestamp, err := parseTimestamp(record.Timestamp)
		if err != nil {
			return err
		}

		activity.CompletedAt = timestamp
		updateToolDuration(activity)
	}

	return nil
}
func getOrCreateToolActivity(
	session *Session,
	indexes map[string]int,
	callID string,
) int {
	if index, exists := indexes[callID]; exists {
		return index
	}

	index := len(session.ToolActivities)

	session.ToolActivities = append(
		session.ToolActivities,
		ToolActivity{
			CallID: callID,
		},
	)

	indexes[callID] = index

	return index
}

func updateToolDuration(activity *ToolActivity) {
	if activity.StartedAt.IsZero() ||
		activity.CompletedAt.IsZero() {
		return
	}

	if activity.CompletedAt.Before(activity.StartedAt) {
		return
	}

	activity.Duration = activity.CompletedAt.Sub(activity.StartedAt)
}
func parseTimestamp(value string) (time.Time, error) {
	if value == "" {
		return time.Time{}, nil
	}

	return time.Parse(time.RFC3339Nano, value)
}
