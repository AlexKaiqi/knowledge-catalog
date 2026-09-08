package cli

import (
	apphome "kc/home"
	"kc/index"
	"kc/kernel"
	"kc/knowledge"
	"kc/snapshot"
)

// RepositoryReadiness keeps publication, declaration and search readiness
// separate. A missing projection is never reported as an empty search result.
type RepositoryReadiness struct {
	Publication     string           `json:"publication"`
	PublishedCommit kernel.CommitID  `json:"publishedCommit,omitempty"`
	Profile         string           `json:"profile"`
	SchemaCount     *int             `json:"schemaCount,omitempty"`
	Search          string           `json:"search"`
	SearchBasis     kernel.CommitID  `json:"searchBasis,omitempty"`
	Reason          kernel.ErrorCode `json:"reason,omitempty"`
}

type ManagedRepositoryDetail struct {
	ManagedRepositorySummary
	Readiness RepositoryReadiness `json:"readiness"`
}

func describeRepositoryReadiness(cx *invocation, item apphome.ManagedRepositoryResult) RepositoryReadiness {
	out := RepositoryReadiness{Publication: "UNAVAILABLE", Profile: "UNAVAILABLE", Search: "UNAVAILABLE"}
	if cx.WS.Reader == nil {
		out.Reason = kernel.ErrCapabilityUnsatisfied
		return out
	}
	repo, err := cx.WS.Reader.Require(kernel.RepositoryID(item.RepositoryID), kernel.ErrCapabilityUnsatisfied)
	if err != nil {
		out.Reason = readinessErrorCode(err)
		return out
	}
	commit, err := repo.Head(snapshot.DefaultRef)
	if err != nil {
		out.Reason = readinessErrorCode(err)
		return out
	}
	out.Publication, out.PublishedCommit = "PUBLISHED", commit
	out.Profile = "MISSING"
	if resolution, err := repo.Resolve(knowledge.SourceProfileObjectID, commit); err != nil {
		out.Profile = "UNAVAILABLE"
	} else if resolution.Status == knowledge.StatusResolved {
		out.Profile = "PRESENT"
	}
	if schemas, ok := repo.(knowledge.SchemaStore); ok {
		if ids, err := schemas.SchemaObjectIDs(commit); err == nil {
			count := len(ids)
			out.SchemaCount = &count
		}
	}
	if err := authorize(cx.Home, "projection.read", map[string]FlagValue{"as": cx.flag("as"), "repo": item.RepositoryID, "catalog": item.Catalog}, cx.Observation.authorization); err != nil {
		out.Reason = readinessErrorCode(err)
		if kernel.CodeOf(err) == kernel.ErrForbidden {
			out.Search = "NOT_AUTHORIZED"
		}
		return out
	}
	if cx.WS.Index == nil {
		out.Search = "NOT_CONFIGURED"
		return out
	}
	meta, err := cx.WS.Index.CheckSearchProjectionAt(repo, commit)
	out.SearchBasis = meta.Basis
	if err != nil {
		out.Reason = readinessErrorCode(err)
		switch meta.State {
		case index.ProjectionStateBuilding, index.ProjectionStateUpdating,
			index.ProjectionStateFailed, index.ProjectionStateRetired:
			out.Search = meta.State
		case "":
			if out.Reason == kernel.ErrCapabilityUnsatisfied {
				out.Search = "NOT_READY"
			}
		}
		return out
	}
	out.Search = "READY"
	return out
}

func readinessErrorCode(err error) kernel.ErrorCode {
	if code := kernel.CodeOf(err); code != "" {
		return code
	}
	return kernel.ErrTemporaryUnavailable
}
