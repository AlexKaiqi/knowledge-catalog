// Package storecontract holds pure storage DTOs shared by the root contract
// and private backends. It has no implementations or application semantics.
package storecontract

import "kc/kernel"

// CommitMetadata contains immutable literal commit provenance.
type CommitMetadata struct {
	Commit    kernel.CommitID `json:"commit"`
	Author    string          `json:"author,omitempty"`
	Message   string          `json:"message,omitempty"`
	RequestID string          `json:"requestId,omitempty"`
	RuleID    string          `json:"ruleId,omitempty"`
}

type DirectoryRequest struct {
	Commit       kernel.CommitID `json:"commit"`
	Directory    string          `json:"directory,omitempty"`
	Limit        int             `json:"limit,omitempty"`
	Continuation string          `json:"continuation,omitempty"`
}

type DirectoryEntry struct {
	Name string `json:"name"`
	Kind string `json:"kind"` // file or directory
}

type DirectoryPage struct {
	Entries      []DirectoryEntry `json:"entries"`
	Continuation string           `json:"continuation,omitempty"`
	Exhausted    bool             `json:"exhausted"`
	Generation   string           `json:"generation"`
}

type TreeChange struct {
	Path    string `json:"path"`
	Content []byte `json:"content,omitempty"`
	Remove  bool   `json:"remove,omitempty"`
}

type TreeChangeSet struct {
	TargetRepository     kernel.RepositoryID `json:"targetRepository"`
	TargetRef            string              `json:"targetRef"`
	BaseCommit           kernel.CommitID     `json:"baseCommit"`
	ExpectedTargetCommit kernel.CommitID     `json:"expectedTargetCommit"`
	Changes              []TreeChange        `json:"changes"`
	Message              string              `json:"message,omitempty"`
	Author               string              `json:"author,omitempty"`
	RequestID            string              `json:"requestId,omitempty"`
	RuleID               string              `json:"ruleId,omitempty"`
}

const DefaultRef = "refs/heads/main"

func RefOrDefault(ref string) string {
	if ref == "" {
		return DefaultRef
	}
	return ref
}
