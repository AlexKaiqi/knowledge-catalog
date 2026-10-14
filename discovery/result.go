package discovery

import (
	"kc/kernel"
	objectmodel "kc/object"
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
	ProviderOrder []any      `json:"-"` // executor-only ordering tuple; not part of the public result protocol
	MatchedFields []FieldRef `json:"matchedFields,omitempty"`
}

type SearchView struct {
	Snapshots           map[kernel.RepositoryID]kernel.CommitID `json:"snapshots"`
	ProjectionRevisions map[kernel.RepositoryID]string          `json:"projectionRevisions,omitempty"`
}

type UnitVersion struct {
	Address           objectmodel.Address      `json:"address"`
	Digest            kernel.Digest            `json:"digest"`
	DeclarationDigest kernel.Digest            `json:"declarationDigest"`
	SchemaRef         string                   `json:"schemaRef,omitempty"`
	ValueSource       *objectmodel.ValueSource `json:"valueSource,omitempty"`
	ValueBasis        string                   `json:"valueBasis"`
}

type ObjectVersion struct {
	Repository        kernel.RepositoryID           `json:"repository"`
	ObjectID          objectmodel.ObjectID          `json:"objectId"`
	DeclarationCommit kernel.CommitID               `json:"declarationCommit"`
	Units             []UnitVersion                 `json:"units"`
	Observations      []objectmodel.UnitObservation `json:"observations,omitempty"`
}

type ObjectHit struct {
	Object   objectmodel.ObjectValue `json:"object"`
	Version  ObjectVersion           `json:"version"`
	Evidence []LaneEvidence          `json:"evidence"`
}

type SearchResult struct {
	RetrievalEvidenceID string               `json:"retrievalEvidenceId,omitempty"`
	SearchView          SearchView           `json:"searchView"`
	Completeness        Completeness         `json:"completeness"`
	Claims              []string             `json:"claims,omitempty"`
	Hits                []ObjectHit          `json:"hits"`
	Continuation        string               `json:"continuation,omitempty"`
	Stats               SearchExecutionStats `json:"-"`
}

func VersionOf(value objectmodel.ObjectValue) ObjectVersion {
	version := ObjectVersion{
		Repository: value.Repository, ObjectID: value.Address.ObjectID,
		DeclarationCommit: value.Commit, Units: []UnitVersion{},
	}
	declarations := value.Declarations
	if len(declarations) == 0 {
		declarations = []objectmodel.UnitDeclaration{{Address: value.Address, Digest: kernel.CanonicalDigest(value.Value)}}
	}
	for _, declaration := range declarations {
		basis := "snapshot:" + string(value.Commit)
		if declaration.ValueSource != nil && declaration.ValueSource.Kind == objectmodel.ValueSourceBinding {
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
