package projection

import "kc/kernel"

const (
	IndexModeIncremental = "incremental"
	IndexModeRebuild     = "rebuild"
	IndexModeReady       = "ready"

	IndexCauseContent  = "content"
	IndexCauseSchema   = "schema"
	IndexCauseReady    = "ready"
	IndexCauseCold     = "cold"
	IndexCauseDiverged = "diverged"
)

// IndexSync reports how a projection caught up to a commit.
type IndexSync struct {
	Mode           string              `json:"mode"`
	Cause          string              `json:"cause"`
	Repository     kernel.RepositoryID `json:"repository"`
	BasisCommit    kernel.CommitID     `json:"basisCommit"`
	AccessDigest   kernel.Digest       `json:"accessDigest"`
	PhysicalDigest kernel.Digest       `json:"physicalDigest"`
	ObjectCount    int                 `json:"objectCount"`
	Updated        int                 `json:"updated"`
	Removed        int                 `json:"removed"`
}
