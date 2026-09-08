// Package integrationruntime hosts explicitly built provider integrations
// outside KC Server. Source access remains in the integration executable;
// publication crosses the existing typed Writer API.
package integrationruntime

import (
	"context"
	"time"

	"kc/connector"
	"kc/kernel"
	"kc/knowledge"
)

type ArtifactSpec struct {
	ID            string   `json:"id"`
	Directory     string   `json:"directory"`
	Build         []string `json:"build,omitempty"`
	Executable    string   `json:"executable"`
	CredentialEnv []string `json:"credentialEnv,omitempty"`
}

type Artifact struct {
	ID            string    `json:"id"`
	Executable    string    `json:"executable"`
	Digest        string    `json:"digest"`
	CredentialEnv []string  `json:"credentialEnv,omitempty"`
	BuiltAt       time.Time `json:"builtAt"`
}

type Manifest struct {
	ID              string              `json:"id"`
	ArtifactID      string              `json:"artifactId"`
	Server          string              `json:"server"`
	Repository      kernel.RepositoryID `json:"repository"`
	Ref             string              `json:"ref,omitempty"`
	Mode            connector.Mode      `json:"mode"`
	Scope           connector.Scope     `json:"scope"`
	IntervalSeconds int                 `json:"intervalSeconds"`
	TimeoutSeconds  int                 `json:"timeoutSeconds,omitempty"`
}

type Checkpoint struct {
	Cursor string          `json:"cursor,omitempty"`
	Commit kernel.CommitID `json:"commit,omitempty"`
}

type CollectInput struct {
	Manifest   Manifest        `json:"manifest"`
	Checkpoint Checkpoint      `json:"checkpoint"`
	BaseCommit kernel.CommitID `json:"baseCommit"`
}

// Collection is the executable's stdout. Target, mode and scope are fixed by
// Manifest. Observed must be read at CollectInput.BaseCommit; source mapping
// and the knowledge addresses remain the provider integration's responsibility.
type Collection struct {
	Desired    []connector.Unit     `json:"desired"`
	Observed   []connector.Observed `json:"observed"`
	SourceRefs []string             `json:"sourceRefs"`
	Cursor     string               `json:"cursor,omitempty"`
	ProducedAt string               `json:"producedAt,omitempty"`
}

type Writer interface {
	Head(context.Context, kernel.RepositoryID, string) (kernel.CommitID, error)
	Commit(context.Context, string, knowledge.CommitChangeSet) (kernel.CommitID, error)
}

// WriterFactory authenticates against the manifest's saved Server and returns
// its verified principal, never a caller-asserted user identity.
type WriterFactory func(context.Context, string) (Writer, string, error)

type Pending struct {
	CommandID string                    `json:"commandId"`
	ChangeSet knowledge.CommitChangeSet `json:"changeSet"`
	Cursor    string                    `json:"cursor,omitempty"`
}

type state struct {
	Manifest   Manifest   `json:"manifest"`
	Artifact   Artifact   `json:"artifact"`
	Owner      string     `json:"owner"`
	Active     bool       `json:"active"`
	Phase      string     `json:"phase"`
	Checkpoint Checkpoint `json:"checkpoint"`
	Pending    *Pending   `json:"pending,omitempty"`
	LastError  string     `json:"lastError,omitempty"`
	LastRunAt  time.Time  `json:"lastRunAt,omitempty"`
	NextRunAt  time.Time  `json:"nextRunAt,omitempty"`
	LeaseID    string     `json:"leaseId,omitempty"`
	LeaseUntil time.Time  `json:"leaseUntil,omitempty"`
	Runs       uint64     `json:"runs"`
}

// Status omits collected values, ChangeSet bodies and all credentials. It can
// be inspected while the source, KC Server or the current login is unavailable.
type Status struct {
	Manifest         Manifest   `json:"manifest"`
	Owner            string     `json:"owner"`
	Active           bool       `json:"active"`
	Phase            string     `json:"phase"`
	Checkpoint       Checkpoint `json:"checkpoint"`
	PendingCommandID string     `json:"pendingCommandId,omitempty"`
	LastError        string     `json:"lastError,omitempty"`
	LastRunAt        time.Time  `json:"lastRunAt,omitempty"`
	NextRunAt        time.Time  `json:"nextRunAt,omitempty"`
	Runs             uint64     `json:"runs"`
}
