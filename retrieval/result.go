package retrieval

import (
	"kc/kernel"
	"kc/knowledge"
)

type Completeness string

const (
	CompletenessComplete Completeness = "complete"
	CompletenessPartial  Completeness = "partial"
)

type LaneEvidence struct {
	Provider      string     `json:"provider"`
	Lane          string     `json:"lane"`
	Guarantee     string     `json:"guarantee"`
	LocalRank     int        `json:"localRank,omitempty"`
	LocalScore    float64    `json:"localScore,omitempty"`
	MatchedFields []FieldRef `json:"matchedFields,omitempty"`
}

type SearchView struct {
	Snapshots map[kernel.RepositoryID]kernel.CommitID `json:"snapshots"`
}

type UnitVersion struct {
	Address           knowledge.Address      `json:"address"`
	Digest            kernel.Digest          `json:"digest"`
	DeclarationDigest kernel.Digest          `json:"declarationDigest"`
	SchemaRef         string                 `json:"schemaRef,omitempty"`
	ValueSource       *knowledge.ValueSource `json:"valueSource,omitempty"`
	ValueBasis        string                 `json:"valueBasis"`
}

type KnowledgeVersion struct {
	Repository        kernel.RepositoryID `json:"repository"`
	ObjectID          knowledge.ObjectID  `json:"objectId"`
	DeclarationCommit kernel.CommitID     `json:"declarationCommit"`
	Units             []UnitVersion       `json:"units"`
}

type KnowledgeHit struct {
	Knowledge knowledge.KnowledgeValue `json:"knowledge"`
	Version   KnowledgeVersion         `json:"version"`
	Evidence  []LaneEvidence           `json:"evidence"`
}

type SearchResult struct {
	SearchView   SearchView     `json:"searchView"`
	Completeness Completeness   `json:"completeness"`
	Claims       []string       `json:"claims,omitempty"`
	Hits         []KnowledgeHit `json:"hits"`
	Continuation string         `json:"continuation,omitempty"`
}

func VersionOf(value knowledge.KnowledgeValue) KnowledgeVersion {
	version := KnowledgeVersion{
		Repository: value.Repository, ObjectID: value.Address.ObjectID,
		DeclarationCommit: value.Commit, Units: []UnitVersion{},
	}
	declarations := value.Declarations
	if len(declarations) == 0 {
		declarations = []knowledge.UnitDeclaration{{Address: value.Address, Digest: kernel.CanonicalDigest(value.Value)}}
	}
	for _, declaration := range declarations {
		basis := "snapshot:" + string(value.Commit)
		if declaration.ValueSource != nil && declaration.ValueSource.Kind == knowledge.ValueSourceBinding {
			basis = "binding-declaration:" + string(value.Commit) + ":" + string(declaration.DeclarationDigest)
		}
		version.Units = append(version.Units, UnitVersion{
			Address: declaration.Address, Digest: declaration.Digest,
			DeclarationDigest: declaration.DeclarationDigest,
			SchemaRef:         declaration.SchemaRef, ValueSource: declaration.ValueSource,
			ValueBasis: basis,
		})
	}
	return version
}
