package observability

import (
	"fmt"
	"strings"

	"kc/kernel"
	"kc/knowledge"
	"kc/snapshot"
)

type KnowledgeAccess struct {
	KnowledgeRef knowledge.PinnedKnowledgeRef `json:"knowledgeRef"`
	Address      *knowledge.Address           `json:"address,omitempty"`
	Observations []knowledge.UnitObservation  `json:"observations,omitempty"`
}

type FileAccess struct {
	FileRef snapshot.FileRef `json:"fileRef"`
}

// SnapshotAccess records a bulk read whose smallest honest target is a pinned snapshot.
type SnapshotAccess struct {
	Repository kernel.RepositoryID `json:"repository"`
	Commit     kernel.CommitID     `json:"commit"`
}

type AccessEvent struct {
	EvidenceID string            `json:"evidenceId,omitempty"`
	OccurredAt string            `json:"occurredAt"`
	Identity   IdentityContext   `json:"identity"`
	Trace      TraceContext      `json:"trace,omitempty"`
	Action     string            `json:"action"`
	RequestID  string            `json:"requestId,omitempty"`
	Workspace  string            `json:"workspace,omitempty"`
	PinID      string            `json:"pinId,omitempty"`
	Decision   string            `json:"decision"`
	RuleID     string            `json:"ruleId,omitempty"`
	Result     string            `json:"result"`
	Knowledge  []KnowledgeAccess `json:"knowledge,omitempty"`
	Files      []FileAccess      `json:"files,omitempty"`
	Snapshots  []SnapshotAccess  `json:"snapshots,omitempty"`
	Stream     string            `json:"stream,omitempty"`
	Cursor     string            `json:"cursor,omitempty"`
	Error      map[string]any    `json:"error,omitempty"`
}

func (e AccessEvent) Validate() error {
	if err := e.Identity.Validate(); err != nil {
		return err
	}
	if err := e.Trace.Validate(); err != nil {
		return err
	}
	if strings.TrimSpace(e.Action) == "" {
		return fmt.Errorf("action is required")
	}
	if e.Decision != "ALLOW" && e.Decision != "DENY" {
		return fmt.Errorf("decision must be ALLOW or DENY")
	}
	if e.Result != "RESOLVED" && e.Result != "ERROR" {
		return fmt.Errorf("result must be RESOLVED or ERROR")
	}
	for _, target := range e.Knowledge {
		ref := target.KnowledgeRef
		if ref.Repository == "" || ref.Commit == "" || ref.Object == "" {
			return fmt.Errorf("knowledge access requires repository, commit, and object")
		}
		for _, observation := range target.Observations {
			if observation.Address.ObjectID == "" || observation.DeclarationCommit == "" || observation.DeclarationDigest == "" {
				return fmt.Errorf("knowledge observation requires Address and declaration commit/digest")
			}
			if observation.Address.ObjectID != ref.Object || observation.DeclarationCommit != ref.Commit {
				return fmt.Errorf("knowledge observation must match the accessed object and declaration commit")
			}
			if err := knowledge.ValidateObservationBasis(observation.Basis); err != nil {
				return err
			}
		}
	}
	for _, target := range e.Files {
		if target.FileRef.Repository == "" || target.FileRef.Commit == "" || target.FileRef.Path == "" {
			return fmt.Errorf("file access requires repository, commit, and path")
		}
	}
	for _, target := range e.Snapshots {
		if target.Repository == "" || target.Commit == "" {
			return fmt.Errorf("snapshot access requires repository and commit")
		}
	}
	return nil
}

type AccessQuery struct {
	EvidenceID   string
	Since        string
	Until        string
	Principal    string
	OnBehalfOf   string
	Action       string
	TraceID      string
	Repository   kernel.RepositoryID
	Object       knowledge.ObjectID
	Limit        int
	Continuation string
}

// AccessPage is one bounded audit window. Limit selects the newest matching
// events; Continuation returns the next older window. CompleteThrough is the
// adapter watermark for events already acknowledged on this store.
type AccessPage struct {
	Entries         []AccessEvent `json:"entries"`
	Continuation    string        `json:"continuation,omitempty"`
	Exhausted       bool          `json:"exhausted"`
	CompleteThrough string        `json:"completeThrough,omitempty"`
}
