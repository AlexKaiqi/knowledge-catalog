package connectorhost

import (
	"encoding/json"
	"time"

	"kc/connector"
)

const (
	APIVersion = "connector.kc/v1alpha1"
	Kind       = "Connector"
)

// Manifest is the only user-declared resource in the MVP. Observer and
// Translator are logical stages inside the command, not independently
// registered plugins.
type Manifest struct {
	APIVersion string   `yaml:"apiVersion" json:"apiVersion"`
	Kind       string   `yaml:"kind" json:"kind"`
	Metadata   Metadata `yaml:"metadata" json:"metadata"`
	Spec       Spec     `yaml:"spec" json:"spec"`
}

type Metadata struct {
	ID          string `yaml:"id" json:"id"`
	Description string `yaml:"description,omitempty" json:"description,omitempty"`
	Owner       string `yaml:"owner,omitempty" json:"owner,omitempty"`
}

type Spec struct {
	Command     []string          `yaml:"command" json:"command"`
	Test        *CommandSpec      `yaml:"test,omitempty" json:"test,omitempty"`
	Maintenance MaintenancePolicy `yaml:"maintenance" json:"maintenance"`
	Target      Target            `yaml:"target" json:"target"`
	Access      *AccessSpec       `yaml:"access,omitempty" json:"access,omitempty"`
	Runtime     RuntimePolicy     `yaml:"runtime,omitempty" json:"runtime,omitempty"`
}

type CommandSpec struct {
	Command []string `yaml:"command" json:"command"`
}

type MaintenancePolicy struct {
	Representation string    `yaml:"representation" json:"representation"`
	Triggers       []Trigger `yaml:"triggers,omitempty" json:"triggers,omitempty"`
	Freshness      string    `yaml:"freshness,omitempty" json:"freshness,omitempty"`
}

type Trigger struct {
	Kind  string `yaml:"kind" json:"kind"`
	Every string `yaml:"every,omitempty" json:"every,omitempty"`
}

type Target struct {
	Repository string `yaml:"repository" json:"repository"`
	Ref        string `yaml:"ref,omitempty" json:"ref,omitempty"`
	Scope      Scope  `yaml:"scope" json:"scope"`
}

type Scope struct {
	Aspects      []string `yaml:"aspects,omitempty" json:"aspects,omitempty"`
	AllowEntity  bool     `yaml:"allowEntity,omitempty" json:"allowEntity,omitempty"`
	ObjectPrefix string   `yaml:"objectPrefix,omitempty" json:"objectPrefix,omitempty"`
}

func (s Scope) Protocol() connector.Scope {
	return connector.Scope{Aspects: append([]string(nil), s.Aspects...), AllowEntity: s.AllowEntity, ObjectPrefix: s.ObjectPrefix}
}

type RuntimePolicy struct {
	Timeout string `yaml:"timeout,omitempty" json:"timeout,omitempty"`
}

// AccessSpec is the optional live-resource entry point shipped in the same
// business integration package as the Collector.
type AccessSpec struct {
	Protocol   string   `yaml:"protocol" json:"protocol"`
	Command    []string `yaml:"command" json:"command"`
	Operations []string `yaml:"operations" json:"operations"`
	Timeout    string   `yaml:"timeout,omitempty" json:"timeout,omitempty"`
}

// RunRequest is written to the connector command on stdin.
type RunRequest struct {
	RunID            string          `json:"runId"`
	ConnectorID      string          `json:"connectorId"`
	GenerationDigest string          `json:"generationDigest"`
	Trigger          RunTrigger      `json:"trigger"`
	TargetBaseCommit string          `json:"targetBaseCommit,omitempty"`
	Checkpoint       json.RawMessage `json:"checkpoint,omitempty"`
}

type RunTrigger struct {
	Kind string   `json:"kind"`
	Keys []string `json:"keys,omitempty"`
	At   string   `json:"at"`
}

// ConnectorOutput is the process boundary. The command observes and
// translates; the Host owns Scope, Preview, Writer and checkpoint advancement.
type ConnectorOutput struct {
	Observation    Observation          `json:"observation"`
	Mode           connector.Mode       `json:"mode"`
	Desired        []connector.Unit     `json:"desired"`
	Observed       []connector.Observed `json:"observed,omitempty"`
	NextCheckpoint json.RawMessage      `json:"nextCheckpoint,omitempty"`
	Message        string               `json:"message,omitempty"`
}

type Observation struct {
	SourceRefs     []string `json:"sourceRefs"`
	ObservedAt     string   `json:"observedAt"`
	Representation string   `json:"representation"`
	Coverage       Coverage `json:"coverage"`
}

type Coverage struct {
	Kind string   `json:"kind"`
	Keys []string `json:"keys,omitempty"`
}

type HostConfig struct {
	Repository string `yaml:"repository" json:"repository"`
	Ref        string `yaml:"ref" json:"ref"`
	RepoPath   string `yaml:"checkoutPath" json:"checkoutPath"`
	SyncEvery  string `yaml:"syncEvery" json:"syncEvery"`
	KCURL      string `yaml:"kcUrl" json:"kcUrl"`
}

type RepositorySyncState struct {
	Repository   string `json:"repository"`
	Ref          string `json:"ref"`
	CheckoutPath string `json:"checkoutPath"`
	Commit       string `json:"commit,omitempty"`
	LastSyncAt   string `json:"lastSyncAt,omitempty"`
	Error        string `json:"error,omitempty"`
}

type ConnectorState struct {
	ConnectorID       string          `json:"connectorId"`
	Active            bool            `json:"active"`
	ActiveGeneration  string          `json:"activeGeneration,omitempty"`
	Checkpoint        json.RawMessage `json:"checkpoint,omitempty"`
	CheckpointVersion uint64          `json:"checkpointVersion"`
	LastRunID         string          `json:"lastRunId,omitempty"`
	LastSuccessAt     string          `json:"lastSuccessAt,omitempty"`
	LastCommit        string          `json:"lastCommit,omitempty"`
	LastError         string          `json:"lastError,omitempty"`
	NextRunAt         string          `json:"nextRunAt,omitempty"`
}

type RunOutcome string

const (
	RunSucceeded RunOutcome = "SUCCEEDED"
	RunFailed    RunOutcome = "FAILED"
	RunPreviewed RunOutcome = "PREVIEWED"
	RunEmpty     RunOutcome = "EMPTY"
)

type RunRecord struct {
	RunID             string            `json:"runId"`
	ConnectorID       string            `json:"connectorId"`
	GenerationDigest  string            `json:"generationDigest"`
	Trigger           RunTrigger        `json:"trigger"`
	PreviewOnly       bool              `json:"previewOnly"`
	StartedAt         string            `json:"startedAt"`
	FinishedAt        string            `json:"finishedAt"`
	Outcome           RunOutcome        `json:"outcome"`
	Summary           connector.Summary `json:"summary"`
	CommandID         string            `json:"commandId,omitempty"`
	TargetCommit      string            `json:"targetCommit,omitempty"`
	CheckpointVersion uint64            `json:"checkpointVersion"`
	Error             string            `json:"error,omitempty"`
	Stderr            string            `json:"stderr,omitempty"`
}

type ResourceDescriptorCoordinate struct {
	ObjectID   string `json:"objectId"`
	Repository string `json:"repository"`
	Commit     string `json:"commit"`
}

type AccessIdentity struct {
	Principal       string `json:"principal"`
	Agent           string `json:"agent,omitempty"`
	Session         string `json:"session,omitempty"`
	ParentSession   string `json:"parentSession,omitempty"`
	DelegationDepth int    `json:"delegationDepth,omitempty"`
	RequestID       string `json:"requestId"`
}

// AccessRequest is accepted by the Host. Identity is populated from trusted
// HTTP headers, never from this JSON body.
type AccessRequest struct {
	Descriptor ResourceDescriptorCoordinate `json:"descriptor"`
	Runtime    string                       `json:"runtime"`
	Protocol   string                       `json:"protocol"`
	Operation  string                       `json:"operation"`
	Input      json.RawMessage              `json:"input,omitempty"`
}

// RuntimeAccessRequest is written to the integration package's access command.
type RuntimeAccessRequest struct {
	Descriptor ResourceDescriptorCoordinate `json:"descriptor"`
	Operation  string                       `json:"operation"`
	Input      json.RawMessage              `json:"input,omitempty"`
	Identity   AccessIdentity               `json:"identity"`
}

type AccessResponse struct {
	TraceID    string                       `json:"traceId"`
	Descriptor ResourceDescriptorCoordinate `json:"descriptor"`
	Runtime    string                       `json:"runtime"`
	Generation string                       `json:"generation"`
	Operation  string                       `json:"operation"`
	Result     json.RawMessage              `json:"result"`
}

type AccessTrace struct {
	TraceID      string                       `json:"traceId"`
	StartedAt    string                       `json:"startedAt"`
	FinishedAt   string                       `json:"finishedAt"`
	Identity     AccessIdentity               `json:"identity"`
	Descriptor   ResourceDescriptorCoordinate `json:"descriptor"`
	Runtime      string                       `json:"runtime"`
	Generation   string                       `json:"generation,omitempty"`
	Operation    string                       `json:"operation"`
	InputDigest  string                       `json:"inputDigest"`
	ResultDigest string                       `json:"resultDigest,omitempty"`
	ResultBytes  int                          `json:"resultBytes,omitempty"`
	Error        string                       `json:"error,omitempty"`
}

type ConnectorInfo struct {
	Manifest   Manifest       `json:"manifest"`
	Path       string         `json:"path"`
	Principal  string         `json:"principal"`
	Generation string         `json:"generation"`
	Valid      bool           `json:"valid"`
	Error      string         `json:"error,omitempty"`
	State      ConnectorState `json:"state"`
}

func nowString(t time.Time) string { return t.UTC().Format(time.RFC3339Nano) }
