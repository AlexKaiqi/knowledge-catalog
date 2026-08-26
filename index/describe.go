package index

import (
	"kc/kernel"
	"kc/knowledge"
	"kc/retrieval"
)

// IndexDescriptor describes one working projection, not a published snapshot.
type IndexDescriptor struct {
	BasisRepository  kernel.RepositoryID     `json:"basisRepository"`
	BasisCommit      kernel.CommitID         `json:"basisCommit"`
	ObjectCount      int                     `json:"objectCount"`
	HeadCommit       kernel.CommitID         `json:"headCommit"`
	LagBehindHead    bool                    `json:"lagBehindHead"`
	AccessDigest     kernel.Digest           `json:"accessDigest,omitempty"`
	PhysicalDigest   kernel.Digest           `json:"physicalDigest,omitempty"`
	ProviderRevision string                  `json:"providerRevision,omitempty"`
	Generation       string                  `json:"generation,omitempty"`
	State            string                  `json:"state,omitempty"`
	Coverage         float64                 `json:"coverage,omitempty"`
	Mode             string                  `json:"mode,omitempty"`
	Cause            string                  `json:"cause,omitempty"`
	Schemas          []knowledge.ObjectID    `json:"schemas,omitempty"`
	Lanes            []string                `json:"lanes,omitempty"`
	Fields           []retrieval.AccessField `json:"fields,omitempty"`
}

func (idx *Index) Describe(repo knowledge.Repository) (IndexDescriptor, error) {
	return idx.describe(repo, "")
}

// DescribeAt reports the projection at a frozen commit.
func (idx *Index) DescribeAt(repo knowledge.Repository, commit kernel.CommitID) (IndexDescriptor, error) {
	if commit == "" {
		return idx.Describe(repo)
	}
	if _, err := idx.EnsureAt(repo, commit); err != nil {
		return IndexDescriptor{}, err
	}
	return idx.describe(repo, commit)
}

func (idx *Index) describe(repo knowledge.Repository, commit kernel.CommitID) (IndexDescriptor, error) {
	var (
		eng Engine
		err error
	)
	if commit == "" {
		eng, err = idx.engine(repo.ID())
	} else {
		eng, err = idx.engineForCommit(repo.ID(), commit)
	}
	if err != nil {
		return IndexDescriptor{}, err
	}
	meta, err := eng.LoadMeta()
	if err != nil {
		return IndexDescriptor{}, err
	}
	count, err := eng.Count()
	if err != nil {
		return IndexDescriptor{}, err
	}
	head, err := repo.Head("")
	if err != nil {
		return IndexDescriptor{}, err
	}
	desc := IndexDescriptor{
		BasisRepository: repo.ID(), BasisCommit: meta.Basis, ObjectCount: count,
		HeadCommit: head, LagBehindHead: meta.Basis != "" && head != meta.Basis,
		AccessDigest: meta.AccessDigest, PhysicalDigest: meta.PhysicalDigest,
		ProviderRevision: meta.ProviderRevision, Generation: meta.Generation, State: meta.State,
		Coverage: meta.Coverage, Mode: meta.Mode, Cause: meta.Cause,
	}
	if meta.Basis != "" {
		spec, err := specAtCommit(repo, meta.Basis)
		if err != nil {
			return IndexDescriptor{}, err
		}
		desc.Schemas = spec.Schemas
		desc.Lanes = spec.QueryLanes()
		desc.Fields = spec.Fields
	}
	return desc, nil
}
