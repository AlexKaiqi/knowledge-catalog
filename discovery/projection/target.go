package projection

import "kc/kernel"

type ProjectionTarget struct {
	Repository    kernel.RepositoryID `json:"repository"`
	DesiredCommit kernel.CommitID     `json:"desiredCommit"`
	AppliedCommit kernel.CommitID     `json:"appliedCommit,omitempty"`
	Status        string              `json:"status"`
	LastError     string              `json:"lastError,omitempty"`
	UpdatedAt     string              `json:"updatedAt"`
}

const (
	TargetPending = "PENDING"
	TargetReady   = "READY"
	TargetFailed  = "FAILED"
)
