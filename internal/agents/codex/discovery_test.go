package codex

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDiscoverRolloutsFindsActiveAndArchivedSessions(t *testing.T) {
	home := t.TempDir()

	activeDir := filepath.Join(home, "sessions", "2026", "11", "17")
	archivedDir := filepath.Join(home, "archived_sessions")

	for _, dir := range []string{activeDir, archivedDir} {
		if err := os.MkdirAll(dir, 0700); err != nil {
			t.Fatalf("MkdirAll() failed: %v", err)
		}
	}

	files := []string{
		filepath.Join(activeDir, "rollout-active.jsonl"),
		filepath.Join(archivedDir, "rollout-archived.jsonl"),
		filepath.Join(activeDir, "other.jsonl"),
	}

	for _, path := range files {
		if err := os.WriteFile(path, []byte("{}\n"), 0600); err != nil {
			t.Fatalf("WriteFile() failed: %v", err)
		}
	}

	rollouts, err := DiscoverRollouts(home)
	if err != nil {
		t.Fatalf("DiscoverRollouts() failed: %v", err)
	}

	if len(rollouts) != 2 {
		t.Fatalf("expected 2 rollouts, got %d", len(rollouts))
	}

	for _, rollout := range rollouts {
		if rollout.Size == 0 {
			t.Error("expected non-zero file size")
		}

		if rollout.ModTimeNS == 0 {
			t.Error("expected modification timestamp")
		}
	}
}

func TestDiscoverRolloutsHandlesMissingSessionDirectories(t *testing.T) {
	home := t.TempDir()

	rollouts, err := DiscoverRollouts(home)
	if err != nil {
		t.Fatalf("DiscoverRollouts() failed: %v", err)
	}

	if len(rollouts) != 0 {
		t.Errorf("expected 0 rollouts, got %d", len(rollouts))
	}
}

func TestDiscoverRolloutsRejectsMissingCodexHome(t *testing.T) {
	_, err := DiscoverRollouts(filepath.Join(t.TempDir(), "missing"))

	if err == nil {
		t.Fatal("expected error for missing Codex home")
	}
}
