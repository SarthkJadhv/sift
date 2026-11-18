package codex

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// RolloutFile contains filesystem metadata for a Codex rollout.
type RolloutFile struct {
	Path      string
	Size      int64
	ModTimeNS int64
}

// DiscoverRollouts finds Codex rollout files in active and archived sessions.
// An empty codexHome uses CODEX_HOME or the default ~/.codex directory.
func DiscoverRollouts(codexHome string) ([]RolloutFile, error) {
	if strings.TrimSpace(codexHome) == "" {
		codexHome = os.Getenv("CODEX_HOME")
	}

	if strings.TrimSpace(codexHome) == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("resolve user home: %w", err)
		}

		codexHome = filepath.Join(home, ".codex")
	}

	codexHome, err := filepath.Abs(codexHome)
	if err != nil {
		return nil, fmt.Errorf("resolve Codex home path: %w", err)
	}

	info, err := os.Stat(codexHome)
	if err != nil {
		return nil, fmt.Errorf("access Codex home: %w", err)
	}

	if !info.IsDir() {
		return nil, errors.New("Codex home is not a directory")
	}

	var rollouts []RolloutFile

	directories := []string{
		"sessions",
		"archived_sessions",
	}

	for _, directory := range directories {
		root := filepath.Join(codexHome, directory)

		_, err := os.Stat(root)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("access %s: %w", directory, err)
		}

		err = filepath.WalkDir(root, func(
			path string,
			entry fs.DirEntry,
			walkErr error,
		) error {
			if walkErr != nil {
				return walkErr
			}

			if entry.IsDir() {
				return nil
			}

			name := entry.Name()

			if !strings.HasPrefix(name, "rollout-") ||
				filepath.Ext(name) != ".jsonl" {
				return nil
			}

			fileInfo, err := entry.Info()
			if err != nil {
				return fmt.Errorf("read rollout metadata: %w", err)
			}

			if !fileInfo.Mode().IsRegular() {
				return nil
			}

			absolutePath, err := filepath.Abs(path)
			if err != nil {
				return fmt.Errorf("resolve rollout path: %w", err)
			}

			rollouts = append(rollouts, RolloutFile{
				Path:      absolutePath,
				Size:      fileInfo.Size(),
				ModTimeNS: fileInfo.ModTime().UnixNano(),
			})

			return nil
		})

		if err != nil {
			return nil, fmt.Errorf("scan %s: %w", directory, err)
		}
	}

	sort.Slice(rollouts, func(i, j int) bool {
		if rollouts[i].ModTimeNS != rollouts[j].ModTimeNS {
			return rollouts[i].ModTimeNS < rollouts[j].ModTimeNS
		}

		return rollouts[i].Path < rollouts[j].Path
	})

	return rollouts, nil
}
