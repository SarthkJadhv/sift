CREATE TABLE codex_sessions (
    id TEXT PRIMARY KEY,
    project_id TEXT NOT NULL,
    rollout_path TEXT NOT NULL UNIQUE,
    started_at TEXT NOT NULL,
    working_directory TEXT NOT NULL DEFAULT '',
    cli_version TEXT NOT NULL DEFAULT '',
    model_provider TEXT NOT NULL DEFAULT '',
    originator TEXT NOT NULL DEFAULT '',
    thread_source TEXT NOT NULL DEFAULT '',
    file_size_bytes INTEGER NOT NULL DEFAULT 0,
    file_mod_time_ns INTEGER NOT NULL DEFAULT 0,
    last_imported_at TEXT NOT NULL,

    FOREIGN KEY (project_id)
        REFERENCES projects(id)
        ON DELETE CASCADE,

    CHECK(length(trim(id)) > 0),
    CHECK(length(trim(rollout_path)) > 0),
    CHECK(file_size_bytes >= 0)
);

CREATE INDEX idx_codex_sessions_project_started
    ON codex_sessions(project_id, started_at DESC);


CREATE TABLE codex_turns (
    session_id TEXT NOT NULL,
    turn_id TEXT NOT NULL,
    model TEXT NOT NULL DEFAULT '',
    effort TEXT NOT NULL DEFAULT '',
    started_at TEXT NOT NULL DEFAULT '',
    completed_at TEXT NOT NULL DEFAULT '',
    duration_ns INTEGER NOT NULL DEFAULT 0,
    time_to_first_token_ns INTEGER NOT NULL DEFAULT 0,
    model_context_window INTEGER NOT NULL DEFAULT 0,

    PRIMARY KEY (session_id, turn_id),

    FOREIGN KEY (session_id)
        REFERENCES codex_sessions(id)
        ON DELETE CASCADE,

    CHECK(length(trim(turn_id)) > 0),
    CHECK(duration_ns >= 0),
    CHECK(time_to_first_token_ns >= 0),
    CHECK(model_context_window >= 0)
);


CREATE TABLE codex_response_usage (
    session_id TEXT NOT NULL,
    response_id TEXT NOT NULL,
    thread_id TEXT NOT NULL DEFAULT '',
    turn_id TEXT NOT NULL DEFAULT '',
    root_turn_id TEXT NOT NULL DEFAULT '',
    timestamp TEXT NOT NULL DEFAULT '',
    ordinal INTEGER NOT NULL DEFAULT 0,

    input_tokens INTEGER NOT NULL DEFAULT 0,
    cached_input_tokens INTEGER NOT NULL DEFAULT 0,
    cache_write_input_tokens INTEGER NOT NULL DEFAULT 0,
    output_tokens INTEGER NOT NULL DEFAULT 0,
    reasoning_output_tokens INTEGER NOT NULL DEFAULT 0,
    total_tokens INTEGER NOT NULL DEFAULT 0,

    turn_input_tokens INTEGER NOT NULL DEFAULT 0,
    turn_cached_input_tokens INTEGER NOT NULL DEFAULT 0,
    turn_cache_write_input_tokens INTEGER NOT NULL DEFAULT 0,
    turn_output_tokens INTEGER NOT NULL DEFAULT 0,
    turn_reasoning_output_tokens INTEGER NOT NULL DEFAULT 0,
    turn_total_tokens INTEGER NOT NULL DEFAULT 0,

    session_input_tokens INTEGER NOT NULL DEFAULT 0,
    session_cached_input_tokens INTEGER NOT NULL DEFAULT 0,
    session_cache_write_input_tokens INTEGER NOT NULL DEFAULT 0,
    session_output_tokens INTEGER NOT NULL DEFAULT 0,
    session_reasoning_output_tokens INTEGER NOT NULL DEFAULT 0,
    session_total_tokens INTEGER NOT NULL DEFAULT 0,

    PRIMARY KEY (session_id, response_id),

    FOREIGN KEY (session_id)
        REFERENCES codex_sessions(id)
        ON DELETE CASCADE,

    CHECK(length(trim(response_id)) > 0),
    CHECK(input_tokens >= 0),
    CHECK(output_tokens >= 0),
    CHECK(total_tokens >= 0),
    CHECK(session_total_tokens >= 0)
);

CREATE INDEX idx_codex_usage_turn
    ON codex_response_usage(session_id, turn_id);


CREATE TABLE codex_tool_activities (
    session_id TEXT NOT NULL,
    call_id TEXT NOT NULL,
    item_id TEXT NOT NULL DEFAULT '',
    name TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL DEFAULT '',
    started_at TEXT NOT NULL DEFAULT '',
    completed_at TEXT NOT NULL DEFAULT '',
    duration_ns INTEGER NOT NULL DEFAULT 0,
    has_output INTEGER NOT NULL DEFAULT 0,
    call_ordinal INTEGER NOT NULL DEFAULT 0,
    output_ordinal INTEGER NOT NULL DEFAULT 0,

    PRIMARY KEY (session_id, call_id),

    FOREIGN KEY (session_id)
        REFERENCES codex_sessions(id)
        ON DELETE CASCADE,

    CHECK(length(trim(call_id)) > 0),
    CHECK(duration_ns >= 0),
    CHECK(has_output IN (0, 1))
);

CREATE INDEX idx_codex_tool_name
    ON codex_tool_activities(session_id, name);