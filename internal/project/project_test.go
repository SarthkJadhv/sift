package project

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/SarthkJadhv/sift/internal/config"
)

func TestInitializeFindsGitRoot(t *testing.T) {
	root := t.TempDir()

	expectedRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}

	// Simulate a Git worktree where .git is a file.
	if err := os.WriteFile(
		filepath.Join(root, ".git"),
		[]byte("gitdir: /tmp/example"),
		0600,
	); err != nil {
		t.Fatal(err)
	}

	nested := filepath.Join(root, "internal", "config")

	if err := os.MkdirAll(nested, 0755); err != nil {
		t.Fatal(err)
	}

	cfg, created, err := Initialize(nested)
	if err != nil {
		t.Fatalf("Initialize() returned error: %v", err)
	}

	if !created {
		t.Fatal("expected project to be created")
	}

	if cfg.RootPath != expectedRoot {
		t.Errorf("expected root %s, got %s", expectedRoot, cfg.RootPath)
	}

	if cfg.Name != filepath.Base(root) {
		t.Errorf("unexpected project name: %s", cfg.Name)
	}
}

func TestInitializeFallsBackToStartDirectory(t *testing.T) {
	root := t.TempDir()

	expectedRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}

	cfg, created, err := Initialize(root)
	if err != nil {
		t.Fatalf("Initialize() returned error: %v", err)
	}

	if !created {
		t.Fatal("expected project to be created")
	}

	if cfg.RootPath != expectedRoot {
		t.Errorf("expected root %s, got %s", expectedRoot, cfg.RootPath)
	}
}

func TestInitializeIsIdempotent(t *testing.T) {
	root := t.TempDir()

	first, created, err := Initialize(root)
	if err != nil {
		t.Fatalf("first Initialize() failed: %v", err)
	}

	if !created {
		t.Fatal("expected first initialization to create project")
	}

	second, created, err := Initialize(root)
	if err != nil {
		t.Fatalf("second Initialize() failed: %v", err)
	}

	if created {
		t.Fatal("expected second initialization to reuse project")
	}

	if first.ProjectID != second.ProjectID {
		t.Fatal("project ID changed on repeated initialization")
	}

	if !first.CreatedAt.Equal(second.CreatedAt) {
		t.Fatal("creation timestamp changed")
	}

	configPath := filepath.Join(first.RootPath, ".sift", "config.json")

	stored, err := config.Load(configPath)
	if err != nil {
		t.Fatalf("failed to load stored config: %v", err)
	}

	if stored.ProjectID != first.ProjectID {
		t.Fatal("stored project ID does not match")
	}
}

func TestInitializeRejectsFilePath(t *testing.T) {
	root := t.TempDir()
	filePath := filepath.Join(root, "not-a-directory")

	if err := os.WriteFile(filePath, []byte("test"), 0600); err != nil {
		t.Fatal(err)
	}

	_, _, err := Initialize(filePath)

	if err == nil {
		t.Fatal("expected error for file path")
	}
}
