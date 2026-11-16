package appdata

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

type Paths struct {
	RootDir    string
	Database   string
	BackupsDir string
}

// Resolve returns the paths used by SIFT for local data storage.
func Resolve() (Paths, error) {
	var paths Paths

	baseDir, err := os.UserConfigDir()
	if err != nil {
		return paths, fmt.Errorf(
			"resolve user config directory: %w", err,
		)
	}

	if baseDir == "" {
		return paths, errors.New("user config directory is empty")
	}

	return Paths{
		RootDir:    filepath.Join(baseDir, "sift"),
		Database:   filepath.Join(baseDir, "sift", "sift.db"),
		BackupsDir: filepath.Join(baseDir, "sift", "backups"),
	}, nil
}

// Ensure creates the required SIFT data directories.
func (p Paths) Ensure() error {
	if p.RootDir == "" || p.BackupsDir == "" || p.Database == "" {
		return errors.New("invalid SIFT application paths")
	}

	if err := os.MkdirAll(p.RootDir, 0700); err != nil {
		return fmt.Errorf("create SIFT data directory: %w", err)
	}

	if err := os.MkdirAll(p.BackupsDir, 0700); err != nil {
		return fmt.Errorf("create SIFT backups directory: %w", err)
	}

	return nil
}
