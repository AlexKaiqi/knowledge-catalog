package connectorhost

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"
	"time"

	"kc/connector"
	"kc/kernel"
)

type Host struct {
	store  *Store
	config HostConfig
	kc     KCClient
	now    func() time.Time

	mu      sync.Mutex
	running map[string]bool
}

func OpenHost(store *Store) (*Host, error) {
	config, err := store.LoadConfig()
	if err != nil {
		return nil, err
	}
	return NewHost(store, config, KCClient{BaseURL: config.KCURL, Principal: config.Principal}), nil
}

func NewHost(store *Store, config HostConfig, client KCClient) *Host {
	return &Host{store: store, config: config, kc: client, now: time.Now, running: map[string]bool{}}
}

func (h *Host) Connectors(ctx context.Context, runTests bool) ([]ConnectorInfo, error) {
	loaded, err := Discover(h.config.RepoPath)
	if err != nil {
		return nil, err
	}
	out := make([]ConnectorInfo, 0, len(loaded))
	for _, item := range loaded {
		if runTests {
			if err := ValidateConnector(ctx, item); err != nil {
				return nil, fmt.Errorf("connector %s: %w", item.Manifest.Metadata.ID, err)
			}
		}
		state, err := h.store.LoadState(item.Manifest.Metadata.ID)
		if err != nil {
			return nil, err
		}
		out = append(out, ConnectorInfo{Manifest: item.Manifest, Path: item.Dir, Generation: item.Generation, State: state})
	}
	return out, nil
}

func (h *Host) Connector(id string) (LoadedConnector, error) {
	loaded, err := Discover(h.config.RepoPath)
	if err != nil {
		return LoadedConnector{}, err
	}
	for _, item := range loaded {
		if item.Manifest.Metadata.ID == id {
			return item, nil
		}
	}
	return LoadedConnector{}, fmt.Errorf("unknown connector %q", id)
}

func (h *Host) Activate(ctx context.Context, id string) (ConnectorState, error) {
	loaded, err := h.Connector(id)
	if err != nil {
		return ConnectorState{}, err
	}
	if err := ValidateConnector(ctx, loaded); err != nil {
		return ConnectorState{}, err
	}
	state, err := h.store.LoadState(id)
	if err != nil {
		return ConnectorState{}, err
	}
	state.Active = true
	state.ActiveGeneration = loaded.Generation
	state.LastError = ""
	if interval := connectorInterval(loaded.Manifest); interval > 0 {
		state.NextRunAt = nowString(h.now().Add(interval))
	} else {
		state.NextRunAt = ""
	}
	if err := h.store.SaveState(state); err != nil {
		return ConnectorState{}, err
	}
	return state, nil
}

func (h *Host) Pause(id string) (ConnectorState, error) {
	state, err := h.store.LoadState(id)
	if err != nil {
		return ConnectorState{}, err
	}
	state.Active = false
	state.NextRunAt = ""
	if err := h.store.SaveState(state); err != nil {
		return ConnectorState{}, err
	}
	return state, nil
}

func (h *Host) Run(ctx context.Context, id string, trigger RunTrigger, previewOnly, scheduled bool) (record RunRecord, err error) {
	if !h.acquire(id) {
		return RunRecord{}, fmt.Errorf("connector %s is already running", id)
	}
	defer h.release(id)

	loaded, err := h.Connector(id)
	if err != nil {
		return RunRecord{}, err
	}
	state, err := h.store.LoadState(id)
	if err != nil {
		return RunRecord{}, err
	}
	if scheduled {
		if !state.Active {
			return RunRecord{}, fmt.Errorf("connector %s is paused", id)
		}
		if state.ActiveGeneration != loaded.Generation {
			state.LastError = "connector files changed after activation; validate and activate the new generation"
			_ = h.store.SaveState(state)
			return RunRecord{}, fmt.Errorf("connector %s generation changed after activation", id)
		}
	}
	start := h.now()
	if trigger.Kind == "" {
		trigger.Kind = "manual"
	}
	if trigger.At == "" {
		trigger.At = nowString(start)
	}
	record = RunRecord{
		RunID: newRunID(), ConnectorID: id, GenerationDigest: loaded.Generation,
		Trigger: trigger, PreviewOnly: previewOnly, StartedAt: nowString(start),
		CheckpointVersion: state.CheckpointVersion,
	}
	defer func() {
		record.FinishedAt = nowString(h.now())
		if err != nil {
			record.Outcome = RunFailed
			record.Error = err.Error()
			state.LastError = err.Error()
		}
		state.LastRunID = record.RunID
		if interval := connectorInterval(loaded.Manifest); state.Active && interval > 0 {
			state.NextRunAt = nowString(h.now().Add(interval))
		}
		_ = h.store.SaveState(state)
		_ = h.store.AppendRun(record)
	}()

	base, err := h.kc.RepoHead(ctx, loaded.Manifest.Spec.Target.Repository)
	if err != nil {
		return record, err
	}
	request := RunRequest{
		RunID: record.RunID, ConnectorID: id, GenerationDigest: loaded.Generation,
		Trigger: trigger, TargetBaseCommit: base, Checkpoint: cloneRaw(state.Checkpoint),
	}
	output, stderr, err := executeConnector(ctx, loaded, request)
	record.Stderr = stderr
	if err != nil {
		return record, err
	}
	if err := validateOutput(loaded.Manifest, output); err != nil {
		return record, err
	}
	targetRef := loaded.Manifest.Spec.Target.Ref
	if targetRef == "" {
		targetRef = "refs/heads/main"
	}
	preview, err := connector.Preview(connector.Plan{
		ConnectorID: id, Mode: output.Mode, Scope: loaded.Manifest.Spec.Target.Scope.Protocol(),
		TargetRepository: kernel.RepositoryID(loaded.Manifest.Spec.Target.Repository),
		TargetRef:        targetRef, BaseCommit: kernel.CommitID(base), Desired: output.Desired,
		Observed: output.Observed, SourceRefs: output.Observation.SourceRefs,
		ProducedAt: output.Observation.ObservedAt, ActorRef: "connector/" + id, Message: output.Message,
	})
	if err != nil {
		return record, err
	}
	record.Summary = preview.Summary
	if previewOnly {
		record.Outcome = RunPreviewed
		return record, nil
	}
	if preview.Empty {
		record.Outcome = RunEmpty
		advanceCheckpoint(&state, output.NextCheckpoint, "", h.now())
		record.CheckpointVersion = state.CheckpointVersion
		return record, nil
	}
	commandID := connector.CommandID(id, connector.RunKey(preview.ChangeSet.Operations))
	record.CommandID = commandID
	receipt, err := h.kc.Commit(ctx, commandID, preview.ChangeSet)
	if err != nil {
		return record, err
	}
	record.TargetCommit = string(receipt.Result.NewCommit)
	record.Outcome = RunSucceeded
	advanceCheckpoint(&state, output.NextCheckpoint, record.TargetCommit, h.now())
	record.CheckpointVersion = state.CheckpointVersion
	return record, nil
}

func advanceCheckpoint(state *ConnectorState, next json.RawMessage, commit string, now time.Time) {
	if len(bytes.TrimSpace(next)) > 0 && string(bytes.TrimSpace(next)) != "null" {
		state.Checkpoint = cloneRaw(next)
		state.CheckpointVersion++
	}
	state.LastSuccessAt = nowString(now)
	if commit != "" {
		state.LastCommit = commit
	}
	state.LastError = ""
}

func validateOutput(manifest Manifest, output ConnectorOutput) error {
	if output.Observation.Representation != "STATE" {
		return fmt.Errorf("MVP connector output observation.representation must be STATE")
	}
	if len(output.Observation.SourceRefs) == 0 {
		return fmt.Errorf("observation.sourceRefs is required")
	}
	if _, err := time.Parse(time.RFC3339Nano, output.Observation.ObservedAt); err != nil {
		return fmt.Errorf("observation.observedAt must be RFC3339: %w", err)
	}
	switch output.Observation.Coverage.Kind {
	case "FULL", "KEYED":
	default:
		return fmt.Errorf("MVP observation.coverage.kind must be FULL or KEYED")
	}
	if output.Mode == connector.ModeReconcile && output.Observation.Coverage.Kind != "FULL" {
		return fmt.Errorf("reconcile requires FULL observation coverage")
	}
	if output.Mode != connector.ModePatch && output.Mode != connector.ModeReconcile {
		return fmt.Errorf("connector output mode must be patch or reconcile")
	}
	for _, unit := range output.Desired {
		if !manifest.Spec.Target.Scope.Protocol().Contains(unit.Address) {
			return fmt.Errorf("desired address is outside manifest scope")
		}
	}
	return nil
}

func executeConnector(parent context.Context, loaded LoadedConnector, request RunRequest) (ConnectorOutput, string, error) {
	timeout := 5 * time.Minute
	if raw := loaded.Manifest.Spec.Runtime.Timeout; raw != "" {
		timeout, _ = time.ParseDuration(raw)
	}
	ctx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()
	body, err := json.Marshal(request)
	if err != nil {
		return ConnectorOutput{}, "", err
	}
	command := loaded.Manifest.Spec.Command
	cmd := exec.CommandContext(ctx, command[0], command[1:]...)
	cmd.Dir = loaded.Dir
	cmd.Env = append(os.Environ(), "KC_CONNECTOR_ID="+request.ConnectorID, "KC_CONNECTOR_RUN_ID="+request.RunID)
	cmd.Stdin = bytes.NewReader(append(body, '\n'))
	var stdout, stderr cappedBuffer
	stdout.limit = 8 << 20
	stderr.limit = 1 << 20
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return ConnectorOutput{}, stderr.String(), fmt.Errorf("connector command timed out: %w", ctx.Err())
		}
		return ConnectorOutput{}, stderr.String(), fmt.Errorf("connector command failed: %w", err)
	}
	var output ConnectorOutput
	dec := json.NewDecoder(bytes.NewReader(stdout.Bytes()))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&output); err != nil {
		return ConnectorOutput{}, stderr.String(), fmt.Errorf("decode connector output: %w", err)
	}
	return output, stderr.String(), nil
}

type cappedBuffer struct {
	bytes.Buffer
	limit int64
}

func (b *cappedBuffer) Write(p []byte) (int, error) {
	if int64(b.Len()+len(p)) > b.limit {
		return 0, fmt.Errorf("process output exceeds %d bytes", b.limit)
	}
	return b.Buffer.Write(p)
}

func (h *Host) acquire(id string) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.running[id] {
		return false
	}
	h.running[id] = true
	return true
}

func (h *Host) release(id string) {
	h.mu.Lock()
	delete(h.running, id)
	h.mu.Unlock()
}

func cloneRaw(value json.RawMessage) json.RawMessage {
	if value == nil {
		return nil
	}
	return append(json.RawMessage(nil), value...)
}

func newRunID() string {
	var raw [8]byte
	if _, err := io.ReadFull(rand.Reader, raw[:]); err != nil {
		return fmt.Sprintf("run-%d", time.Now().UnixNano())
	}
	return "run-" + hex.EncodeToString(raw[:])
}

func (h *Host) Tick(ctx context.Context) {
	items, err := h.Connectors(ctx, false)
	if err != nil {
		return
	}
	now := h.now()
	for _, item := range items {
		if !item.State.Active || item.State.NextRunAt == "" {
			continue
		}
		due, err := time.Parse(time.RFC3339Nano, item.State.NextRunAt)
		if err != nil || due.After(now) {
			continue
		}
		id := item.Manifest.Metadata.ID
		go func() {
			_, _ = h.Run(ctx, id, RunTrigger{Kind: "schedule", At: nowString(h.now())}, false, true)
		}()
	}
}

func (h *Host) ServeScheduler(ctx context.Context) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			h.Tick(ctx)
		}
	}
}

var _ io.Writer = (*cappedBuffer)(nil)
