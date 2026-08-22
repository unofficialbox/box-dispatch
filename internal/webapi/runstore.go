package webapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

const runHistoryFilename = "web-runs.json"

// runStore keeps the browser's non-sensitive activity history locally. It is
// intentionally separate from deployment audits: an audit is the execution
// record, while this store also preserves validation-only and failed runs.
type runStore interface {
	Load() ([]persistedRun, error)
	Save([]persistedRun) error
}

type persistedRunStore struct {
	path string
}

func defaultRunStore() runStore {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return unavailableRunStore{err: err}
	}
	return persistedRunStore{path: filepath.Join(configDir, "dispatch", runHistoryFilename)}
}

func (s persistedRunStore) Load() ([]persistedRun, error) {
	data, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read web run history: %w", err)
	}
	var runs []persistedRun
	if err := json.Unmarshal(data, &runs); err != nil {
		return nil, fmt.Errorf("decode web run history: %w", err)
	}
	return runs, nil
}

func (s persistedRunStore) Save(runs []persistedRun) error {
	data, err := json.MarshalIndent(runs, "", "  ")
	if err != nil {
		return fmt.Errorf("encode web run history: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return fmt.Errorf("create web run history directory: %w", err)
	}
	temporary, err := os.CreateTemp(filepath.Dir(s.path), ".web-runs-*.tmp")
	if err != nil {
		return fmt.Errorf("create web run history file: %w", err)
	}
	temporaryName := temporary.Name()
	defer os.Remove(temporaryName)
	if err := temporary.Chmod(0o600); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("protect web run history file: %w", err)
	}
	if _, err := temporary.Write(data); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("write web run history: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close web run history: %w", err)
	}
	if err := os.Rename(temporaryName, s.path); err != nil {
		return fmt.Errorf("save web run history: %w", err)
	}
	return nil
}

type unavailableRunStore struct {
	err error
}

func (s unavailableRunStore) Load() ([]persistedRun, error) { return nil, s.err }
func (s unavailableRunStore) Save([]persistedRun) error     { return s.err }

type memoryRunStore struct{}

func (memoryRunStore) Load() ([]persistedRun, error) { return nil, nil }
func (memoryRunStore) Save([]persistedRun) error     { return nil }
