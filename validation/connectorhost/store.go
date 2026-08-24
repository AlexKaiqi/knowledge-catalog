package connectorhost

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"gopkg.in/yaml.v3"
)

type Store struct {
	home string
	mu   sync.Mutex
}

func NewStore(home string) (*Store, error) {
	abs, err := filepath.Abs(home)
	if err != nil {
		return nil, err
	}
	for _, dir := range []string{abs, filepath.Join(abs, "state"), filepath.Join(abs, "runs")} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return nil, err
		}
	}
	return &Store{home: abs}, nil
}

func (s *Store) AppendAccessTrace(trace AccessTrace) error {
	body, err := json.Marshal(trace)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	f, err := os.OpenFile(filepath.Join(s.home, "access.jsonl"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	if _, err := f.Write(append(body, '\n')); err != nil {
		return err
	}
	return f.Sync()
}

func (s *Store) AccessTraces(limit int) ([]AccessTrace, error) {
	f, err := os.Open(filepath.Join(s.home, "access.jsonl"))
	if errors.Is(err, os.ErrNotExist) {
		return []AccessTrace{}, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var traces []AccessTrace
	scanner := bufio.NewScanner(f)
	buffer := make([]byte, 64*1024)
	scanner.Buffer(buffer, 2*1024*1024)
	for scanner.Scan() {
		var trace AccessTrace
		if err := json.Unmarshal(scanner.Bytes(), &trace); err != nil {
			return nil, err
		}
		traces = append(traces, trace)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	sort.Slice(traces, func(i, j int) bool { return traces[i].StartedAt > traces[j].StartedAt })
	if limit > 0 && len(traces) > limit {
		traces = traces[:limit]
	}
	return traces, nil
}

func (s *Store) Home() string { return s.home }

func (s *Store) SaveConfig(config HostConfig) error {
	if stringsTrim(config.RepoPath) == "" || stringsTrim(config.KCURL) == "" {
		return fmt.Errorf("checkoutPath and kcUrl are required")
	}
	abs, err := filepath.Abs(config.RepoPath)
	if err != nil {
		return err
	}
	config.RepoPath = abs
	if stringsTrim(config.Repository) == "" {
		// Direct package tests may point at an existing checkout. Production
		// configuration always sets the authoritative Git repository.
		config.Repository = abs
	} else {
		config.Repository = NormalizeRepositoryLocation(config.Repository)
	}
	if config.Ref == "" {
		config.Ref = "refs/heads/main"
	}
	if config.SyncEvery == "" {
		config.SyncEvery = "30s"
	}
	if every, err := time.ParseDuration(config.SyncEvery); err != nil || every <= 0 {
		return fmt.Errorf("syncEvery must be a positive duration")
	}
	body, err := yaml.Marshal(config)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return atomicWrite(filepath.Join(s.home, "config.yaml"), body, 0o600)
}

func (s *Store) LoadConfig() (HostConfig, error) {
	body, err := os.ReadFile(filepath.Join(s.home, "config.yaml"))
	if err != nil {
		return HostConfig{}, err
	}
	var config HostConfig
	if err := yaml.Unmarshal(body, &config); err != nil {
		return HostConfig{}, err
	}
	return config, nil
}

func (s *Store) LoadState(id string) (ConnectorState, error) {
	path, err := s.statePath(id)
	if err != nil {
		return ConnectorState{}, err
	}
	body, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return ConnectorState{ConnectorID: id}, nil
	}
	if err != nil {
		return ConnectorState{}, err
	}
	var state ConnectorState
	if err := json.Unmarshal(body, &state); err != nil {
		return ConnectorState{}, err
	}
	if state.ConnectorID != id {
		return ConnectorState{}, fmt.Errorf("state connector id mismatch")
	}
	return state, nil
}

func (s *Store) SaveState(state ConnectorState) error {
	path, err := s.statePath(state.ConnectorID)
	if err != nil {
		return err
	}
	body, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	body = append(body, '\n')
	s.mu.Lock()
	defer s.mu.Unlock()
	return atomicWrite(path, body, 0o600)
}

func (s *Store) AppendRun(run RunRecord) error {
	if err := safeID(run.ConnectorID); err != nil {
		return err
	}
	body, err := json.Marshal(run)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	f, err := os.OpenFile(filepath.Join(s.home, "runs", run.ConnectorID+".jsonl"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	if _, err := f.Write(append(body, '\n')); err != nil {
		return err
	}
	return f.Sync()
}

func (s *Store) Runs(id string, limit int) ([]RunRecord, error) {
	if err := safeID(id); err != nil {
		return nil, err
	}
	f, err := os.Open(filepath.Join(s.home, "runs", id+".jsonl"))
	if errors.Is(err, os.ErrNotExist) {
		return []RunRecord{}, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var runs []RunRecord
	scanner := bufio.NewScanner(f)
	buffer := make([]byte, 64*1024)
	scanner.Buffer(buffer, 2*1024*1024)
	for scanner.Scan() {
		var run RunRecord
		if err := json.Unmarshal(scanner.Bytes(), &run); err != nil {
			return nil, err
		}
		runs = append(runs, run)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	sort.Slice(runs, func(i, j int) bool { return runs[i].StartedAt > runs[j].StartedAt })
	if limit > 0 && len(runs) > limit {
		runs = runs[:limit]
	}
	return runs, nil
}

func (s *Store) statePath(id string) (string, error) {
	if err := safeID(id); err != nil {
		return "", err
	}
	return filepath.Join(s.home, "state", id+".json"), nil
}

func safeID(id string) error {
	if id == "" || stringsTrim(id) != id {
		return fmt.Errorf("unsafe connector id")
	}
	for _, r := range id {
		if !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' || r == '.') {
			return fmt.Errorf("unsafe connector id %q", id)
		}
	}
	return nil
}

func stringsTrim(value string) string {
	start, end := 0, len(value)
	for start < end && (value[start] == ' ' || value[start] == '\t' || value[start] == '\n' || value[start] == '\r') {
		start++
	}
	for end > start && (value[end-1] == ' ' || value[end-1] == '\t' || value[end-1] == '\n' || value[end-1] == '\r') {
		end--
	}
	return value[start:end]
}

func atomicWrite(path string, body []byte, mode os.FileMode) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".tmp-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if err := tmp.Chmod(mode); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(body); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpPath, path)
}
