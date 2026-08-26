package index

import (
	"kc/kernel"
	"kc/retrieval"
)

// ProjectionSpec identifies one provider's discardable physical realization
// of a logical AccessSpec. It is not stored in Schema or Workspace recipes.
type ProjectionSpec struct {
	Repository       kernel.RepositoryID `json:"repository"`
	BasisCommit      kernel.CommitID     `json:"basisCommit"`
	AccessDigest     kernel.Digest       `json:"accessDigest"`
	Provider         string              `json:"provider"`
	ProviderRevision string              `json:"providerRevision,omitempty"`
	PhysicalDigest   kernel.Digest       `json:"physicalDigest,omitempty"`
}

// RetrievalFragment is the provider-specific portion of one request. The
// reference implementation currently selects one managed provider per member;
// every clause is still probed independently so unsupported or inexact work is
// visible before execution.
type RetrievalFragment struct {
	Provider   string                  `json:"provider"`
	Search     retrieval.SearchRequest `json:"search"`
	Capability Capability              `json:"capability"`
}

// RetrievalPlan is ephemeral and request-scoped. It separates the logical
// AccessSpec from physical ProjectionSpec and records the guarantee used to
// derive public completeness claims.
type RetrievalPlan struct {
	SearchView retrieval.SearchView    `json:"searchView"`
	Access     retrieval.AccessSpec    `json:"access"`
	Projection ProjectionSpec          `json:"projection"`
	Search     retrieval.SearchRequest `json:"search"`
	Fragments  []RetrievalFragment     `json:"fragments"`
}

func PlanRetrieval(retriever Retriever, identity ProviderIdentity, request retrieval.SearchRequest, access retrieval.AccessSpec) (RetrievalPlan, error) {
	resolved, err := retrieval.ResolveSearch(request, access)
	if err != nil {
		return RetrievalPlan{}, err
	}
	provider := "anonymous"
	projection := ProjectionSpec{Repository: access.Repository, BasisCommit: access.Commit, AccessDigest: access.AccessDigest}
	if identity != nil {
		provider = identity.ProviderID()
		projection.Provider = provider
		projection.ProviderRevision = identity.ProviderRevision()
		projection.PhysicalDigest = identity.PhysicalDigest()
	} else {
		projection.Provider = provider
	}
	fragments := make([]RetrievalFragment, 0, len(resolved.Clauses))
	for _, clause := range resolved.Clauses {
		fragmentSearch := retrieval.SearchOf(clause)
		fragments = append(fragments, RetrievalFragment{
			Provider: provider, Search: fragmentSearch,
			Capability: retriever.Probe(clause, access),
		})
	}
	return RetrievalPlan{
		SearchView: retrieval.SearchView{Snapshots: map[kernel.RepositoryID]kernel.CommitID{access.Repository: access.Commit}},
		Access:     access, Projection: projection, Search: resolved,
		Fragments: fragments,
	}, nil
}
