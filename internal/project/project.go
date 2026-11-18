package project

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/SarthkJadhv/sift/internal/config"
)

func Initialize(startDir string) (config.ProjectConfig, bool, error) {
	var cfg config.ProjectConfig

	root, err := FindRoot(startDir)
	if err != nil {
		return cfg, false, err
	}

	configPath := filepath.Join(root, ".sift", "config.json")

	// Check whether the project is already initialized.
	existing, err := config.Load(configPath)

	if err == nil {
		if filepath.Clean(existing.RootPath) != filepath.Clean(root) {
			return cfg, false, errors.New(
				"existing configuration belongs to a different project root",
			)
		}

		return existing, false, nil
	}

	if !errors.Is(err, os.ErrNotExist) {
		return cfg, false, fmt.Errorf(
			"load existing project config: %w", err,
		)
	}

	// Generate a unique project identifier.
	id, err := newProjectID()
	if err != nil {
		return cfg, false, err
	}

	cfg = config.ProjectConfig{
		SchemaVersion: config.CurrentSchemaVersion,
		ProjectID:     id,
		Name:          filepath.Base(root),
		RootPath:      root,
		CreatedAt:     time.Now().UTC(),
	}

	// Persist project configuration.
	if err := config.Save(configPath, cfg); err != nil {
		return cfg, false, fmt.Errorf(
			"save project configuration: %w", err,
		)
	}

	return cfg, true, nil
}

// FindRoot searches upwards for a Git repository.
// If no Git repository exists, it uses the supplied directory.
func FindRoot(startDir string) (string, error) {
	root, err := filepath.Abs(startDir)
	if err != nil {
		return "", fmt.Errorf("resolve project path: %w", err)
	}

	root, err = filepath.EvalSymlinks(root)
	if err != nil {
		return "", fmt.Errorf("resolve project symlinks: %w", err)
	}

	info, err := os.Stat(root)
	if err != nil {
		return "", fmt.Errorf("inspect project directory: %w", err)
	}

	if !info.IsDir() {
		return "", errors.New("project path is not a directory")
	}

	startRoot := root

	for {
		gitPath := filepath.Join(root, ".git")

		_, err := os.Stat(gitPath)

		if err == nil {
			return root, nil
		}

		if !errors.Is(err, os.ErrNotExist) {
			return "", fmt.Errorf("inspect git metadata: %w", err)
		}

		parent := filepath.Dir(root)

		if parent == root {
			break
		}

		root = parent
	}

	return startRoot, nil
}

// newProjectID generates a UUID v4 using the standard library.
func newProjectID() (string, error) {
	var id [16]byte

	if _, err := rand.Read(id[:]); err != nil {
		return "", fmt.Errorf("generate project ID: %w", err)
	}

	// Set UUID version to 4.
	id[6] = (id[6] & 0x0f) | 0x40

	// Set RFC 4122 variant bits.
	id[8] = (id[8] & 0x3f) | 0x80

	encoded := hex.EncodeToString(id[:])

	return fmt.Sprintf(
		"%s-%s-%s-%s-%s",
		encoded[0:8],
		encoded[8:12],
		encoded[12:16],
		encoded[16:20],
		encoded[20:32],
	), nil
}
