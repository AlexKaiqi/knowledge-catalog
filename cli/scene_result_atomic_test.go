package cli_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"sync"
	"testing"
	"time"
)

func TestSceneResultConcurrentWriters(t *testing.T) {
	node, run := t.TempDir(), t.TempDir()
	t.Setenv("KC_VALIDATION_RUN_DIR", run)
	const workers, writes = 8, 8
	start := make(chan struct{})
	results := make(chan error, workers*writes)
	var done sync.WaitGroup
	for worker := range workers {
		done.Add(1)
		go func() {
			defer done.Done()
			<-start
			for index := range writes {
				results <- writeSceneResult(node, concurrentSceneResult(worker, index))
			}
		}()
	}
	close(start)
	done.Wait()
	close(results)
	for err := range results {
		if err != nil {
			t.Errorf("concurrent result publication: %v", err)
		}
	}
	assertConcurrentSceneResults(t, node, run, workers, writes)
}

func TestSceneResultConcurrentProcesses(t *testing.T) {
	const processNode = "KC_SCENE_RESULT_PROCESS_NODE"
	const processID = "KC_SCENE_RESULT_PROCESS_ID"
	const workers, writes = 2, 32
	if node := os.Getenv(processNode); node != "" {
		worker, err := strconv.Atoi(os.Getenv(processID))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(node, fmt.Sprintf("ready-%d", worker)), nil, 0o600); err != nil {
			t.Fatal(err)
		}
		waitForSceneResultPath(t, filepath.Join(node, "start"))
		for index := range writes {
			if err := writeSceneResult(node, concurrentSceneResult(worker, index)); err != nil {
				t.Errorf("process result publication: %v", err)
			}
		}
		return
	}
	node, run := t.TempDir(), t.TempDir()
	t.Setenv("KC_VALIDATION_RUN_DIR", run)
	commands := make([]*exec.Cmd, workers)
	outputs := make([]bytes.Buffer, workers)
	for worker := range workers {
		command := exec.Command(os.Args[0], "-test.run=^TestSceneResultConcurrentProcesses$", "-test.count=1")
		command.Env = append(os.Environ(), processNode+"="+node, processID+"="+strconv.Itoa(worker),
			"KC_ASSERT_E2E_COVERAGE=", "KC_COMMAND_COVERAGE_REPORT=")
		command.Stdout, command.Stderr = &outputs[worker], &outputs[worker]
		if err := command.Start(); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = command.Process.Kill() })
		commands[worker] = command
	}
	for worker := range workers {
		waitForSceneResultPath(t, filepath.Join(node, fmt.Sprintf("ready-%d", worker)))
	}
	if err := os.WriteFile(filepath.Join(node, "start"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	for worker, command := range commands {
		if err := command.Wait(); err != nil {
			t.Errorf("writer process %d: %v\n%s", worker, err, &outputs[worker])
		}
	}
	assertConcurrentSceneResults(t, node, run, workers, writes)
}

func TestSceneResultRenameFailureKeepsOtherFiles(t *testing.T) {
	node, run := t.TempDir(), t.TempDir()
	t.Setenv("KC_VALIDATION_RUN_DIR", run)
	dir := filepath.Join(node, "_results")
	latest := filepath.Join(dir, "latest.json")
	if err := os.MkdirAll(latest, 0o755); err != nil {
		t.Fatal(err)
	}
	sentinel := filepath.Join(latest, "existing")
	otherTemporary := filepath.Join(dir, "latest-other.tmp")
	for _, path := range []string{sentinel, otherTemporary} {
		if err := os.WriteFile(path, []byte("untouched"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	err := writeSceneResult(node, concurrentSceneResult(0, 0))
	var rename *os.LinkError
	if !errors.As(err, &rename) || rename.Op != "rename" || rename.New != latest {
		t.Fatalf("result publication must report its failed rename: %v", err)
	}
	for _, path := range []string{sentinel, otherTemporary} {
		raw, err := os.ReadFile(path)
		if err != nil || string(raw) != "untouched" {
			t.Errorf("failed publication changed another file %s: %q, %v", path, raw, err)
		}
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 {
		t.Fatalf("failed publication leaked its temporary file: %v", entries)
	}
	paths, err := filepath.Glob(filepath.Join(run, "scenes", "concurrent-result", "*.json"))
	if err != nil || len(paths) != 1 {
		t.Fatalf("failed latest publication lost its independent history: %v, %v", paths, err)
	}
}

func TestSceneResultRejectsMissingNodeDirectory(t *testing.T) {
	const helper = "KC_SCENE_RESULT_MISSING_NODE"
	if os.Getenv(helper) == "1" {
		for _, dir := range []string{"", " \t "} {
			if err := writeSceneResult(dir, concurrentSceneResult(0, 0)); err == nil {
				t.Errorf("missing node directory %q published a scene result", dir)
			}
		}
		return
	}
	dir := t.TempDir()
	command := exec.Command(os.Args[0], "-test.run=^TestSceneResultRejectsMissingNodeDirectory$", "-test.count=1")
	command.Dir = dir
	command.Env = append(os.Environ(), helper+"=1", "KC_VALIDATION_RUN_DIR=", "KC_VALIDATION_RUN_ID=", "KC_VALIDATION_SOURCE_FINGERPRINT=", "KC_ASSERT_E2E_COVERAGE=", "KC_COMMAND_COVERAGE_REPORT=")
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("missing-node result guard: %v\n%s", err, output)
	}
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 0 {
		t.Fatalf("invalid scene node leaked result files into the working directory: %v, %v", entries, err)
	}
}

func concurrentSceneResult(worker, index int) sceneRunReport {
	marker := fmt.Sprintf("writer-%d-report-%d", worker, index)
	return sceneRunReport{Test: "concurrent-result", State: "same-node", OK: true,
		StartedAt: marker, FinishedAt: marker, LastStep: marker}
}

func assertConcurrentSceneResults(t *testing.T, node, run string, workers, writes int) {
	t.Helper()
	paths, err := filepath.Glob(filepath.Join(run, "scenes", "concurrent-result", "*.json"))
	if err != nil || len(paths) != workers*writes {
		t.Fatalf("independent histories = %d, want %d: %v", len(paths), workers*writes, err)
	}
	seen := make(map[string]bool)
	for _, path := range paths {
		report := readConcurrentSceneResult(t, path)
		if filepath.Base(path) != report.ExecutionID+".json" || seen[report.LastStep] {
			t.Fatalf("history lost its invocation identity: %#v", report)
		}
		seen[report.LastStep] = true
	}
	for worker := range workers {
		for index := range writes {
			if !seen[concurrentSceneResult(worker, index).LastStep] {
				t.Errorf("history missing writer %d report %d", worker, index)
			}
		}
	}
	latestPath := filepath.Join(node, "_results", "latest.json")
	latest := readConcurrentSceneResult(t, latestPath)
	if !seen[latest.LastStep] {
		t.Fatalf("latest was not written by a real invocation: %#v", latest)
	}
	raw, err := os.ReadFile(latestPath)
	if err != nil {
		t.Fatal(err)
	}
	history, err := os.ReadFile(filepath.Join(run, "scenes", latest.Test, latest.ExecutionID+".json"))
	if err != nil || !bytes.Equal(raw, history) {
		t.Fatalf("latest differs from its own history: %v", err)
	}
	info, err := os.Stat(latestPath)
	if err != nil || info.Mode().Perm() != 0o644 {
		t.Fatalf("latest must remain readable: info=%v error=%v", info, err)
	}
	entries, err := os.ReadDir(filepath.Dir(latestPath))
	if err != nil || len(entries) != 1 {
		t.Fatalf("successful publication leaked temporary files: %v, %v", entries, err)
	}
}

func readConcurrentSceneResult(t *testing.T, path string) sceneRunReport {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var report sceneRunReport
	if err := json.Unmarshal(raw, &report); err != nil {
		t.Fatalf("incomplete result at %s: %v", path, err)
	}
	if report.ExecutionID == "" || report.Test != "concurrent-result" || report.State != "same-node" ||
		!report.OK || report.LastStep != report.StartedAt || report.LastStep != report.FinishedAt {
		t.Fatalf("result mixes invocation bodies at %s: %#v", path, report)
	}
	return report
}

func waitForSceneResultPath(t *testing.T, path string) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(path); err == nil {
			return
		} else if !errors.Is(err, os.ErrNotExist) {
			t.Fatal(err)
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("writer process did not reach barrier %s", path)
}
