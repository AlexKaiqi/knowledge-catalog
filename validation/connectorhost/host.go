package connectorhost

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
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

	repositoryMu sync.RWMutex
	syncMu       sync.RWMutex
	syncState    RepositorySyncState
}

func OpenHost(store *Store) (*Host, error) {
	config, err := store.LoadConfig()
	if err != nil {
		return nil, err
	}
	return NewHost(store, config, KCClient{BaseURL: config.KCURL}), nil
}

func NewHost(store *Store, config HostConfig, client KCClient) *Host {
	return &Host{
		store: store, config: config, kc: client, now: time.Now, running: map[string]bool{},
		syncState: RepositorySyncState{Repository: config.Repository, Ref: config.Ref, CheckoutPath: config.RepoPath},
	}
}

func (h *Host) Connectors(ctx context.Context, runTests bool) ([]ConnectorInfo, error) {
	h.repositoryMu.RLock()
	defer h.repositoryMu.RUnlock()
	entries, err := InspectRepository(h.config.RepoPath)
	if err != nil {
		return nil, err
	}
	out := make([]ConnectorInfo, 0, len(entries))
	for _, entry := range entries {
		if entry.Error != nil {
			out = append(out, ConnectorInfo{
				Manifest: Manifest{Metadata: Metadata{ID: entry.ID}}, Path: entry.Path,
				Principal: ConnectorPrincipal(entry.ID), Valid: false, Error: entry.Error.Error(),
				State: ConnectorState{ConnectorID: entry.ID},
			})
			continue
		}
		item := *entry.Loaded
		if runTests {
			if err := ValidateConnector(ctx, item); err != nil {
				out = append(out, ConnectorInfo{
					Manifest: item.Manifest, Path: item.Dir, Principal: ConnectorPrincipal(item.Manifest.Metadata.ID),
					Generation: item.Generation, Valid: false, Error: err.Error(), State: ConnectorState{ConnectorID: item.Manifest.Metadata.ID},
				})
				continue
			}
		}
		state, err := h.store.LoadState(item.Manifest.Metadata.ID)
		if err != nil {
			return nil, err
		}
		out = append(out, ConnectorInfo{Manifest: item.Manifest, Path: item.Dir, Principal: ConnectorPrincipal(item.Manifest.Metadata.ID), Generation: item.Generation, Valid: true, State: state})
	}
	return out, nil
}

func (h *Host) Connector(id string) (LoadedConnector, error) {
	h.repositoryMu.RLock()
	defer h.repositoryMu.RUnlock()
	return h.connectorUnlocked(id)
}

func (h *Host) connectorUnlocked(id string) (LoadedConnector, error) {
	entries, err := InspectRepository(h.config.RepoPath)
	if err != nil {
		return LoadedConnector{}, err
	}
	for _, entry := range entries {
		if entry.ID != id {
			continue
		}
		if entry.Error != nil {
			return LoadedConnector{}, fmt.Errorf("connector %s is invalid: %w", id, entry.Error)
		}
		return *entry.Loaded, nil
	}
	return LoadedConnector{}, fmt.Errorf("unknown connector %q", id)
}

func (h *Host) Activate(ctx context.Context, id string) (ConnectorState, error) {
	h.repositoryMu.RLock()
	defer h.repositoryMu.RUnlock()
	loaded, err := h.connectorUnlocked(id)
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
	h.repositoryMu.RLock()
	defer h.repositoryMu.RUnlock()

	loaded, err := h.connectorUnlocked(id)
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

	// A shared development repository contains independently governed
	// Connectors. Never let them inherit one broad Host identity: the runtime
	// identity is deterministic and cannot be selected by connector code.
	kc := h.kc
	kc.Principal = ConnectorPrincipal(id)
	base, err := kc.RepoHead(ctx, loaded.Manifest.Spec.Target.Repository)
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
		ProducedAt: output.Observation.ObservedAt, ActorRef: ConnectorPrincipal(id), Message: output.Message,
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
	receipt, err := kc.Commit(ctx, commandID, preview.ChangeSet)
	if err != nil {
		return record, err
	}
	record.TargetCommit = string(receipt.Result.NewCommit)
	record.Outcome = RunSucceeded
	advanceCheckpoint(&state, output.NextCheckpoint, record.TargetCommit, h.now())
	record.CheckpointVersion = state.CheckpointVersion
	return record, nil
}

// Access resolves only a synchronized, active integration generation. The
// ResourceDescriptor is already pinned by the Agent's Workspace read; the Host
// records that coordinate together with trusted identity headers.
func (h *Host) Access(ctx context.Context, request AccessRequest, identity AccessIdentity) (response AccessResponse, err error) {
	start := h.now()
	trace := AccessTrace{
		TraceID: newAccessID(), StartedAt: nowString(start), Identity: identity,
		Descriptor: request.Descriptor, Runtime: request.Runtime, Operation: request.Operation,
		InputDigest: digestJSON(request.Input),
	}
	defer func() {
		trace.FinishedAt = nowString(h.now())
		if err != nil {
			trace.Error = err.Error()
		}
		_ = h.store.AppendAccessTrace(trace)
	}()
	if strings.TrimSpace(identity.RequestID) == "" {
		return response, fmt.Errorf("resource request identity is missing requestId")
	}
	if strings.TrimSpace(request.Descriptor.ObjectID) == "" || strings.TrimSpace(request.Descriptor.Repository) == "" || strings.TrimSpace(request.Descriptor.Commit) == "" {
		return response, fmt.Errorf("descriptor objectId, repository and commit are required")
	}
	if !validConnectorID(request.Runtime) {
		return response, fmt.Errorf("runtime must name a registered integration package")
	}
	h.repositoryMu.RLock()
	defer h.repositoryMu.RUnlock()
	loaded, err := h.connectorUnlocked(request.Runtime)
	if err != nil {
		return response, err
	}
	trace.Generation = loaded.Generation
	state, err := h.store.LoadState(request.Runtime)
	if err != nil {
		return response, err
	}
	if !state.Active || state.ActiveGeneration != loaded.Generation {
		return response, fmt.Errorf("runtime %s generation is not active", request.Runtime)
	}
	spec := loaded.Manifest.Spec.Access
	if spec == nil {
		return response, fmt.Errorf("runtime %s does not provide live resource access", request.Runtime)
	}
	if request.Protocol != spec.Protocol {
		return response, fmt.Errorf("descriptor protocol %q does not match runtime protocol %q", request.Protocol, spec.Protocol)
	}
	if !containsOperation(spec.Operations, request.Operation) {
		return response, fmt.Errorf("runtime %s does not provide operation %s", request.Runtime, request.Operation)
	}
	result, stderr, err := executeAccess(ctx, loaded, RuntimeAccessRequest{
		Descriptor: request.Descriptor, Operation: request.Operation, Input: cloneRaw(request.Input), Identity: identity,
	})
	if err != nil {
		if strings.TrimSpace(stderr) != "" {
			err = fmt.Errorf("%w: %s", err, strings.TrimSpace(stderr))
		}
		return response, err
	}
	trace.ResultDigest = digestJSON(result)
	trace.ResultBytes = len(result)
	return AccessResponse{
		TraceID: trace.TraceID, Descriptor: request.Descriptor, Runtime: request.Runtime,
		Generation: loaded.Generation, Operation: request.Operation, Result: result,
	}, nil
}

func executeAccess(parent context.Context, loaded LoadedConnector, request RuntimeAccessRequest) (json.RawMessage, string, error) {
	spec := loaded.Manifest.Spec.Access
	if spec == nil {
		return nil, "", fmt.Errorf("integration package has no access command")
	}
	timeout := 30 * time.Second
	if spec.Timeout != "" {
		timeout, _ = time.ParseDuration(spec.Timeout)
	}
	ctx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()
	body, err := json.Marshal(request)
	if err != nil {
		return nil, "", err
	}
	cmd := exec.CommandContext(ctx, spec.Command[0], spec.Command[1:]...)
	cmd.Dir = loaded.Dir
	cmd.Env = append(os.Environ(),
		"KC_RESOURCE_RUNTIME="+loaded.Manifest.Metadata.ID,
		"KC_RESOURCE_REQUEST_ID="+request.Identity.RequestID,
	)
	cmd.Stdin = bytes.NewReader(append(body, '\n'))
	var stdout, stderr cappedBuffer
	stdout.limit = 8 << 20
	stderr.limit = 1 << 20
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return nil, stderr.String(), fmt.Errorf("resource access timed out: %w", ctx.Err())
		}
		return nil, stderr.String(), fmt.Errorf("resource access command failed: %w", err)
	}
	raw := bytes.TrimSpace(stdout.Bytes())
	if len(raw) == 0 || !json.Valid(raw) || raw[0] != '{' {
		return nil, stderr.String(), fmt.Errorf("resource access command must return one JSON object")
	}
	return append(json.RawMessage(nil), raw...), stderr.String(), nil
}

func containsOperation(operations []string, requested string) bool {
	for _, operation := range operations {
		if operation == requested {
			return true
		}
	}
	return false
}

func digestJSON(value []byte) string {
	sum := sha256.Sum256(bytes.TrimSpace(value))
	return "sha256:" + hex.EncodeToString(sum[:])
}

func (h *Host) Sync(ctx context.Context) RepositorySyncState {
	h.repositoryMu.Lock()
	defer h.repositoryMu.Unlock()
	h.syncMu.RLock()
	previousCommit := h.syncState.Commit
	h.syncMu.RUnlock()
	commit, err := SyncRepository(ctx, h.config.Repository, h.config.Ref, h.config.RepoPath)
	state := RepositorySyncState{
		Repository: h.config.Repository, Ref: h.config.Ref, CheckoutPath: h.config.RepoPath,
		Commit: commit, LastSyncAt: nowString(h.now()),
	}
	if err != nil {
		state.Commit = previousCommit
		state.Error = err.Error()
	}
	h.syncMu.Lock()
	h.syncState = state
	h.syncMu.Unlock()
	return state
}

func (h *Host) RepositoryState() RepositorySyncState {
	h.syncMu.RLock()
	defer h.syncMu.RUnlock()
	return h.syncState
}

func (h *Host) ServeRepositorySync(ctx context.Context) {
	every, err := time.ParseDuration(h.config.SyncEvery)
	if err != nil || every <= 0 {
		every = 30 * time.Second
	}
	ticker := time.NewTicker(every)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			h.Sync(ctx)
		}
	}
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
	cmd.Env = append(os.Environ(), "KC_CONNECTOR_ID="+request.ConnectorID, "KC_CONNECTOR_PRINCIPAL="+ConnectorPrincipal(request.ConnectorID), "KC_CONNECTOR_RUN_ID="+request.RunID)
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

// ConnectorPrincipal is the least-privilege KC identity assigned by the Host
// to one flat connector package in the shared development repository.
func ConnectorPrincipal(id string) string { return "connector/" + id }

func newRunID() string {
	var raw [8]byte
	if _, err := io.ReadFull(rand.Reader, raw[:]); err != nil {
		return fmt.Sprintf("run-%d", time.Now().UnixNano())
	}
	return "run-" + hex.EncodeToString(raw[:])
}

func newAccessID() string {
	var raw [8]byte
	if _, err := io.ReadFull(rand.Reader, raw[:]); err != nil {
		return fmt.Sprintf("access-%d", time.Now().UnixNano())
	}
	return "access-" + hex.EncodeToString(raw[:])
}

func (h *Host) Tick(ctx context.Context) {
	items, err := h.Connectors(ctx, false)
	if err != nil {
		return
	}
	now := h.now()
	for _, item := range items {
		if !item.Valid || !item.State.Active || item.State.NextRunAt == "" {
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
