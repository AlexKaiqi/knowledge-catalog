package testkit

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"kc/snapshot"
)

type lakeFSCheckpoint struct {
	repository string
	namespace  string
	branches   map[string]string
	metadata   map[string]string
}

// SnapshotStamps checkpoints only repositories created by this fixture. Restore
// is sequential test setup: it resets refs, not immutable commits, so Dataset
// pins and command receipts keep their original versions. Never use it on a
// deployed authority shared with other writers.
func (f *LakeFSLive) SnapshotStamps(home string) (func() error, error) {
	repositories := map[string]bool{}
	err := filepath.WalkDir(home, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || (entry.Name() != "remote.yaml" && entry.Name() != "registry-authority.yaml" && entry.Name() != "catalog-authority.yaml") {
			return nil
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		_, dsn, ok := parseLakeFSStamp(raw)
		if !ok {
			return fmt.Errorf("invalid lakeFS checkpoint authority stamp %s", path)
		}
		f.mu.Lock()
		defer f.mu.Unlock()
		for _, name := range f.created {
			if dsn == f.DSN(name) {
				repositories[name] = true
				return nil
			}
		}
		return fmt.Errorf("cannot checkpoint unowned lakeFS authority %s", dsn)
	})
	if err != nil {
		return nil, err
	}
	if len(repositories) == 0 {
		return nil, fmt.Errorf("lakeFS checkpoint has no owned authority stamps")
	}
	names := make([]string, 0, len(repositories))
	for name := range repositories {
		names = append(names, name)
	}
	sort.Strings(names)
	checkpoints := make([]lakeFSCheckpoint, 0, len(names))
	for _, name := range names {
		state := lakeFSCheckpoint{repository: name}
		if state.namespace, err = f.checkpointNamespace(name); err != nil {
			return nil, err
		}
		if state.branches, err = f.checkpointBranches(name); err != nil {
			return nil, err
		}
		if state.branches[strings.TrimPrefix(string(snapshot.DefaultRef), "refs/heads/")] == "" {
			return nil, fmt.Errorf("lakeFS checkpoint omitted the default branch for %s", name)
		}
		if err = f.checkpointCall(http.MethodGet, name, "/metadata", nil, &state.metadata); err != nil {
			return nil, err
		}
		if state.metadata == nil {
			return nil, fmt.Errorf("lakeFS checkpoint omitted repository properties for %s", name)
		}
		checkpoints = append(checkpoints, state)
	}
	return func() error {
		for _, state := range checkpoints {
			if err := f.restoreCheckpoint(state); err != nil {
				return err
			}
		}
		return nil
	}, nil
}

func (f *LakeFSLive) checkpointNamespace(repository string) (string, error) {
	var result struct {
		ID        string `json:"id"`
		Namespace string `json:"storage_namespace"`
	}
	err := f.checkpointCall(http.MethodGet, repository, "", nil, &result)
	if err == nil && (result.ID != repository || result.Namespace != f.namespace+"/"+repository) {
		err = fmt.Errorf("lakeFS checkpoint repository identity changed: %s", repository)
	}
	return result.Namespace, err
}

func (f *LakeFSLive) checkpointBranches(repository string) (map[string]string, error) {
	branches := map[string]string{}
	after := ""
	seen := map[string]bool{"": true}
	for {
		var page struct {
			Results []struct {
				ID     string `json:"id"`
				Commit string `json:"commit_id"`
			} `json:"results"`
			Pagination struct {
				More bool   `json:"has_more"`
				Next string `json:"next_offset"`
			} `json:"pagination"`
		}
		query := url.Values{"amount": {"1000"}, "after": {after}}
		if err := f.checkpointCall(http.MethodGet, repository, "/branches?"+query.Encode(), nil, &page); err != nil {
			return nil, err
		}
		if page.Results == nil {
			return nil, fmt.Errorf("lakeFS checkpoint omitted branch inventory for %s", repository)
		}
		for _, branch := range page.Results {
			if branch.ID == "" || branch.Commit == "" || branches[branch.ID] != "" {
				return nil, fmt.Errorf("invalid lakeFS checkpoint branch page for %s", repository)
			}
			branches[branch.ID] = branch.Commit
		}
		if !page.Pagination.More {
			return branches, nil
		}
		if seen[page.Pagination.Next] {
			return nil, fmt.Errorf("lakeFS checkpoint branch pagination made no progress")
		}
		after = page.Pagination.Next
		seen[after] = true
	}
}

func (f *LakeFSLive) restoreCheckpoint(state lakeFSCheckpoint) error {
	namespace, err := f.checkpointNamespace(state.repository)
	if err != nil || namespace != state.namespace {
		return fmt.Errorf("restore lakeFS checkpoint identity: %s: %v", state.repository, err)
	}
	current, err := f.checkpointBranches(state.repository)
	if err != nil {
		return err
	}
	for name := range current {
		if _, keep := state.branches[name]; !keep {
			if err := f.checkpointCall(http.MethodDelete, state.repository, "/branches/"+url.PathEscape(name), nil, nil); err != nil {
				return err
			}
		}
	}
	for name, commit := range state.branches {
		if _, exists := current[name]; !exists {
			body := map[string]string{"name": name, "source": commit}
			if err := f.checkpointCall(http.MethodPost, state.repository, "/branches", body, nil); err != nil {
				return err
			}
		}
		// Even an unchanged HEAD may have uncommitted writes from a failed probe.
		query := url.Values{"ref": {commit}, "force": {"true"}}
		if err := f.checkpointCall(http.MethodPut, state.repository, "/branches/"+url.PathEscape(name)+"/hard_reset?"+query.Encode(), nil, nil); err != nil {
			return err
		}
	}
	restored, err := f.checkpointBranches(state.repository)
	if err != nil {
		return err
	}
	if !maps.Equal(restored, state.branches) {
		return fmt.Errorf("lakeFS checkpoint branch restoration did not persist for %s", state.repository)
	}
	return f.restoreCheckpointMetadata(state)
}

func (f *LakeFSLive) restoreCheckpointMetadata(state lakeFSCheckpoint) error {
	var current map[string]string
	if err := f.checkpointCall(http.MethodGet, state.repository, "/metadata", nil, &current); err != nil {
		return err
	}
	if current == nil {
		return fmt.Errorf("lakeFS checkpoint omitted current repository properties for %s", state.repository)
	}
	var extra []string
	for key := range current {
		if _, keep := state.metadata[key]; !keep {
			extra = append(extra, key)
		}
	}
	if len(extra) > 0 {
		if err := f.checkpointCall(http.MethodDelete, state.repository, "/metadata", map[string]any{"keys": extra}, nil); err != nil {
			return err
		}
	}
	if !maps.Equal(current, state.metadata) && len(state.metadata) > 0 {
		if err := f.checkpointCall(http.MethodPost, state.repository, "/metadata", map[string]any{"metadata": state.metadata}, nil); err != nil {
			return err
		}
	}
	var restored map[string]string
	if err := f.checkpointCall(http.MethodGet, state.repository, "/metadata", nil, &restored); err != nil {
		return err
	}
	if restored == nil || !maps.Equal(restored, state.metadata) {
		return fmt.Errorf("lakeFS checkpoint property restoration did not persist for %s", state.repository)
	}
	return nil
}

func (f *LakeFSLive) checkpointCall(method, repository, suffix string, body, result any) error {
	var payload []byte
	if body != nil {
		var err error
		payload, err = json.Marshal(body)
		if err != nil {
			return err
		}
	}
	path := f.origin + "/api/v1/repositories/" + url.PathEscape(repository) + suffix
	request, err := http.NewRequest(method, path, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	user, pass, _ := strings.Cut(f.cred, ":")
	request.SetBasicAuth(user, pass)
	request.Header.Set("Content-Type", "application/json")
	response, err := f.client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	raw, err := io.ReadAll(response.Body)
	if err != nil {
		return err
	}
	if response.StatusCode >= 300 {
		return fmt.Errorf("lakeFS checkpoint %s %s: HTTP %d: %s", method, repository+suffix, response.StatusCode, raw)
	}
	if result != nil {
		return json.Unmarshal(raw, result)
	}
	return nil
}
