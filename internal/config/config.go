package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

const CurrentSchemaVersion = 1

type ProjectConfig struct {
	SchemaVersion int       `json:"schema_version"`
	ProjectID     string    `json:"project_id"`
	Name          string    `json:"name"`
	RootPath      string    `json:"root_path"`
	CreatedAt     time.Time `json:"created_at"`
}

func (c ProjectConfig) Validate() error {
	if c.SchemaVersion != CurrentSchemaVersion {
		return fmt.Errorf(
			"unsupported config schema version: %d",
			c.SchemaVersion,
		)
	}

	if c.ProjectID == "" {
		return errors.New("project ID is required")
	}

	if c.Name == "" {
		return errors.New("project name is required")
	}

	if c.RootPath == "" || !filepath.IsAbs(c.RootPath) {
		return errors.New("project root must be an absolute path")
	}

	if c.CreatedAt.IsZero() {
		return errors.New("creation timestamp is required")
	}

	return nil
}

func Save(path string, cfg ProjectConfig) error {
	if err := cfg.Validate(); err != nil {
		return err
	}

	dir := filepath.Dir(path)

	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("create config directory: %w", err)
	}

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal project config: %w", err)
	}

	tmp, err := os.CreateTemp(dir, ".config-*.tmp")
	if err != nil {
		return fmt.Errorf("create temporary config: %w", err)
	}

	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)

	if err := tmp.Chmod(0600); err != nil {
		tmp.Close()
		return fmt.Errorf("set config permissions: %w", err)
	}

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return fmt.Errorf("write project config: %w", err)
	}

	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return fmt.Errorf("sync project config: %w", err)
	}

	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close project config: %w", err)
	}

	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("save project config: %w", err)
	}

	return nil
}

func Load(path string) (ProjectConfig, error) {
	var cfg ProjectConfig

	data, err := os.ReadFile(path)
	if err != nil {
		return cfg, fmt.Errorf("read project config: %w", err)
	}

	if err := json.Unmarshal(data, &cfg); err != nil {
		return cfg, fmt.Errorf("parse project config: %w", err)
	}

	if err := cfg.Validate(); err != nil {
		return cfg, fmt.Errorf("validate project config: %w", err)
	}

	return cfg, nil
}
