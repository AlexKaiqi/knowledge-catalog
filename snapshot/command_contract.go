package snapshot

import (
	"encoding/json"
	"kc/kernel"
	"time"
)

// CommandLedger owns the shared durable command namespace. Both literal file
// writes and object writes use the same instance and replay semantics.
// Execute retains PENDING on an unknown apply outcome. Only WriteNotApplied
// proves that an apply error can release the claim and permit another attempt.
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
	ApplicationRequest   json.RawMessage `json:"applicationRequest,omitempty"`
	Authority            kernel.Digest   `json:"authority,omitempty"`
}

type CommandEntry struct {
	CommandID string          `json:"commandId"`
	Digest    string          `json:"digest"`
	Status    string          `json:"status"`
	CreatedAt string          `json:"createdAt"`
	UpdatedAt string          `json:"updatedAt"`
	Receipt   json.RawMessage `json:"receipt"`
	Request   CommandRequest  `json:"request,omitempty"`
	Execution string          `json:"execution,omitempty"`
	Phase     string          `json:"phase,omitempty"`
}

// CommandAttempt records the authority boundary before it is crossed. A
// recovered no-change attempt may never transition to a committing attempt.
type CommandAttempt interface {
	SelectNoChange() error
	BeginCommit() error
	NoChangeOnly() bool
}

// StagedCommandLedger is optional. Durable automatic no-change recovery is
// enabled only when the store supports atomic conditional entry replacement.
type StagedCommandLedger interface {
	SupportsStages() bool
	ExecuteStaged(string, string, CommandRequest, func(CommandAttempt) (any, error)) (CommandEntry, bool, error)
}

type ConditionalCommandStore interface {
	// nil expected means absent; nil replacement means delete. A false result
	// with nil error changes nothing; an error may have an unknown outcome.
	// The entire entry, including execution identity, matches.
	CompareAndSwap(string, *CommandEntry, *CommandEntry) (bool, error)
}

const (
	CommandPhaseReserved = "RESERVED"
	CommandPhaseNoChange = "NO_CHANGE"
	CommandPhaseCommit   = "COMMIT_STARTED"
)

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
