# SIFT

### Local-first AI Coding Agent Intelligence & Observability

[![Go](https://img.shields.io/badge/Go-1.24.5-00ADD8?logo=go&logoColor=white)](https://go.dev/)
[![SQLite](https://img.shields.io/badge/Database-SQLite-003B57?logo=sqlite&logoColor=white)](https://sqlite.org/)
[![Status](https://img.shields.io/badge/Status-Active%20Development-orange)]()

**SIFT** is a local-first developer tool that analyzes AI coding agent activity, measures token consumption, and provides visibility into how coding agents interact with software development workflows.

The goal is to help developers understand agent usage, resource consumption, context efficiency, and eventually the engineering outcomes produced by AI-assisted development.

SIFT is not an AI coding agent. It is an intelligence and observability layer designed to work alongside existing coding agents.

---

## Why SIFT?

AI coding agents are becoming an integral part of software development, but understanding their actual usage and effectiveness remains difficult.

Developers need better visibility into questions such as:

- How much are coding agents consuming in input and output tokens?
- Which models and reasoning configurations are being used?
- How frequently are agents invoking tools?
- How does context consumption evolve throughout a session?
- What engineering work is produced during an agent session?
- Where can context usage and agent workflows be improved?

SIFT aims to bring these signals together into one local, codebase-aware intelligence layer.

## Features

### Agent Activity Observability

- Discovers local Codex session files from active and archived session directories.
- Parses Codex JSONL rollout data.
- Extracts session metadata, turns, model configuration and reasoning effort.
- Tracks tool invocations and their corresponding outputs.
- Records turn duration and time-to-first-token metrics.

### Token Analytics

- Tracks input, cached input, output, reasoning and total tokens.
- Maintains per-response usage and cumulative turn/session usage separately.
- Aggregates token consumption by model.
- Provides reasoning-effort usage breakdowns.
- Reports project-level token consumption.

### Session Analytics

- Maintains chronological session history.
- Tracks turns, responses, models and tool activity.
- Aggregates session-level token consumption.
- Reports average turn execution duration.

### Local Persistence

- SQLite-backed local storage.
- Versioned SQL migrations.
- Transactional session imports.
- Idempotent persistence and re-import handling.
- Metadata-based detection of unchanged rollout files.
- Project-scoped session association.

### Context Telemetry

- Extracts context-window snapshots from Codex token-count events.
- Captures active-context token counts and model context-window limits.
- Preserves cumulative session usage separately from active-context measurements.

Context telemetry parsing is implemented. Persistent context analytics and context-efficiency recommendations are under development.

---

## Architecture

SIFT follows a modular monolithic architecture with separate agent adapters, parsing, storage, repositories and CLI layers.

```text
                 Codex CLI
                     |
                     v
              Local JSONL Files
                     |
                     v
             Rollout Discovery
                     |
                     v
               Codex Adapter
                     |
                     v
                JSONL Parser
                     |
                     v
              Project Matching
                     |
                     v
              SQLite Repository
                     |
                     v
              Local SQLite DB
                     |
             +-------+-------+
             |               |
             v               v
         CLI Stats      Session History
```

### Project Structure

```text
sift/
├── cmd/
│   └── sift/
│       └── main.go
├── internal/
│   ├── agents/
│   │   └── codex/
│   │       ├── discovery.go
│   │       ├── parser.go
│   │       ├── parser_test.go
│   │       └── types.go
│   ├── appdata/
│   ├── cli/
│   ├── config/
│   ├── project/
│   └── storage/
│       ├── migrations/
│       ├── repositories/
│       ├── migrations.go
│       └── sqlite.go
└── go.mod
```

## Technology Stack

| Component | Technology |
|---|---|
| Core language | Go |
| CLI framework | Cobra |
| Local database | SQLite |
| SQLite driver | modernc.org/sqlite |
| Agent data format | JSONL |
| Persistence | SQL migrations and transactions |
| Testing | Go testing package |
| Version control | Git and GitHub |

---

## Getting Started

### Prerequisites

- Go 1.24.5 or compatible version.
- Git.
- Codex CLI installed for Codex activity ingestion.

### Clone the repository

```bash
git clone https://github.com/SarthkJadhv/sift.git
cd sift
```

### Download dependencies

```bash
go mod download
```

### Build

```bash
go build -o bin/sift ./cmd/sift
```

### Run tests

```bash
go test ./...
```

### Run SIFT

During development, commands can be executed directly using `go run`:

```bash
go run ./cmd/sift --help
```

---

## CLI Usage

SIFT currently provides a command-line interface.

### Initialize a project

Run this from the repository you want SIFT to track:

```bash
go run ./cmd/sift init
```

This initializes the local project configuration and registers the project in SQLite.

### List registered projects

```bash
go run ./cmd/sift list
```

### Run environment diagnostics

```bash
go run ./cmd/sift doctor
```

### Import Codex activity

```bash
go run ./cmd/sift codex import
```

Discovers active and archived Codex rollout files and imports sessions associated with the current SIFT project.

### View usage statistics

```bash
go run ./cmd/sift codex stats
```

Displays project-level activity, token consumption, model breakdown and reasoning-effort breakdown.

### View session history

```bash
go run ./cmd/sift codex history
```

Limit the number of sessions displayed:

```bash
go run ./cmd/sift codex history --limit 5
```

---

## Data & Privacy

SIFT follows a local-first design.

- Agent data is read from local session files.
- Structured metadata is persisted in a local SQLite database.
- Raw prompts, assistant responses, reasoning content, tool arguments and tool outputs are not persisted by the current Codex ingestion implementation.
- SIFT does not modify Codex configuration or execution behavior.
- No hosted backend or external AI API is required for the current functionality.

The SQLite database is stored under the operating system's user configuration directory.

---

## Roadmap

SIFT is actively evolving beyond usage tracking toward codebase-aware agent intelligence.

### Engineering Intelligence

- Git integration and session-to-commit correlation.
- File-level change tracking.
- Test execution and outcome analysis.
- Engineering activity and session outcome metrics.

### Repository Intelligence

- Tree-sitter-based parsing.
- AST and symbol analysis.
- Import and dependency relationship analysis.
- Codebase structure and repository intelligence.

### Context Intelligence

- Persistent context-window utilization analytics.
- Context consumption patterns.
- Context-efficiency analysis.
- Deterministic context optimization recommendations.

### Agent Integrations

- Support additional AI coding agents.
- Normalize activity across different agent telemetry formats.
- Provide a unified view of agent activity.

### Local Dashboard

- Optional local dashboard.
- Session and token analytics visualizations.
- Repository and context intelligence views.

---

## Engineering Principles

SIFT is being developed around a few core principles:

- **Local-first:** Keep developer data on the local machine.
- **Privacy-conscious:** Persist structured telemetry rather than raw conversation content.
- **Deterministic:** Prefer measurable signals and rule-based analysis over unnecessary LLM calls.
- **Extensible:** Use modular agent adapters to support multiple coding agents.
- **Reliable:** Prioritize transactional persistence, idempotent ingestion and automated testing.
- **Non-invasive:** Observe and recommend without changing how coding agents operate.

---

## Project Status

SIFT is an actively developed personal engineering project.

The initial Codex ingestion and observability pipeline is implemented, including session discovery, JSONL parsing, token analytics, SQLite persistence and CLI reporting.

Repository intelligence, engineering outcome analysis and context-efficiency recommendations are part of the ongoing development roadmap.