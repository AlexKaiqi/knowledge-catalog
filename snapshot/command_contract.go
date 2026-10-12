package snapshot

import (
	"encoding/json"
	"time"
)

// CommandLedger owns the shared durable command namespace. Both literal file
// writes and knowledge writes use the same instance and replay semantics.
type CommandLedger interface {
	Lookup(string) (CommandEntry, bool)
	Entries() []CommandEntry
	Execute(string, string, CommandRequest, func() (any, error)) (CommandEntry, bool, error)
	ResolvePending(string, string, any) (CommandEntry, error)
	AbandonPending(string, string) (CommandEntry, error)
	Prune(time.Time) (int, error)
}

type CommandRequest struct {
	Kind                 string          `json:"kind"`
	RepositoryID         string          `json:"repositoryId,omitempty"`
	TargetRef            string          `json:"targetRef,omitempty"`
	BaseCommit           string          `json:"baseCommit,omitempty"`
	ExpectedTargetCommit string          `json:"expectedTargetCommit,omitempty"`
	OperationCount       int             `json:"operationCount,omitempty"`
	TreeChangeSet        json.RawMessage `json:"rawChangeSet,omitempty"`
}

type CommandEntry struct {
	CommandID string          `json:"commandId"`
	Digest    string          `json:"digest"`
	Status    string          `json:"status"`
	CreatedAt string          `json:"createdAt"`
	UpdatedAt string          `json:"updatedAt"`
	Receipt   json.RawMessage `json:"receipt"`
	Request   CommandRequest  `json:"request,omitempty"`
}

const (
	StatusPending   = "PENDING"
	StatusApplied   = "APPLIED"
	StatusAbandoned = "ABANDONED"
)

type CommandStore interface {
	Ready() error
	Get(commandID string) (CommandEntry, bool, error)
	Put(CommandEntry) error
	Delete(commandID string) error
	List() ([]CommandEntry, error)
}
