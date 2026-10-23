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
	Provider      string                     `json:"provider"`
	Lane          string                     `json:"lane"`
	Guarantee     string                     `json:"guarantee"`
	LocalRank     int                        `json:"localRank,omitempty"`
	LocalScore    float64                    `json:"localScore,omitempty"`
	ProviderOrder []any                      `json:"-"` // executor-only ordering tuple; not part of the public result protocol
	MatchedFields []FieldRef                 `json:"matchedFields,omitempty"`
	Members       []DeclarationParticipation `json:"members,omitempty"`
}

// DeclarationParticipation attributes a candidate to a selected declaration
// member and the role it played at that member's fixed source version.
type DeclarationParticipation struct {
	Repository kernel.RepositoryID `json:"repository"`
	Commit     kernel.CommitID     `json:"commit"`
	Member     objectmodel.Address `json:"member"`
	Role       string              `json:"role"`
}

type SearchView struct {
	Snapshots      map[kernel.RepositoryID]kernel.CommitID `json:"snapshots"`
	IndexRevisions map[kernel.RepositoryID]string          `json:"indexRevisions,omitempty"`
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
	Sources           []objectmodel.SourceBasis     `json:"sources,omitempty"`
}

type ObjectHit struct {
	Object            objectmodel.ObjectValue       `json:"object"`
	Version           ObjectVersion                 `json:"version"`
	Evidence          []LaneEvidence                `json:"evidence"`
	IndexObservations []objectmodel.UnitObservation `json:"indexObservations,omitempty"`
}

type SearchResult struct {
	RetrievalEvidenceID string       `json:"retrievalEvidenceId,omitempty"`
	SearchView          SearchView   `json:"searchView"`
	Completeness        Completeness `json:"completeness"`
	Claims              []string     `json:"claims,omitempty"`
	Hits                []ObjectHit  `json:"hits"`
	Continuation        string       `json:"continuation,omitempty"`
	// Dataset names the revision a Dataset search ran at. Later steps of the
	// same task pass it as --revision to stay on the same frozen sources.
	Dataset *DatasetBasis `json:"dataset,omitempty"`
	// ExternalConditions names each condition answered from indexed external
	// values and how old those observations may be; candidates are rechecked
	// against current observations (discovery 完整性, liveaccess#U22).
	ExternalConditions []ExternalCondition  `json:"externalConditions,omitempty"`
	Stats              SearchExecutionStats `json:"-"`
}

// ExternalCondition is one search condition evaluated on external values as
// observed when the index was built. OldestObservedAt is the earliest
// observation time among the indexed values.
type ExternalCondition struct {
	Field            FieldRef `json:"field"`
	OldestObservedAt string   `json:"oldestObservedAt,omitempty"`
}

// DatasetBasis is the named Dataset revision a consuming request resolved.
type DatasetBasis struct {
	ID       string `json:"id"`
	Revision int    `json:"revision"`
}

func VersionOf(value objectmodel.ObjectValue) ObjectVersion {
	version := ObjectVersion{
		Repository: value.Repository, ObjectID: value.Address.ObjectID,
		DeclarationCommit: value.Commit, Units: []UnitVersion{},
		Sources: append([]objectmodel.SourceBasis(nil), value.Sources...),
	}
	declarations := value.Declarations
	if len(declarations) == 0 {
		declarations = []objectmodel.UnitDeclaration{{Address: value.Address, Digest: kernel.CanonicalDigest(value.Value)}}
	}
	for _, declaration := range declarations {
		commit := observationDeclarationCommit(value, declaration)
		basis := "snapshot:" + string(commit)
		if declaration.ValueSource != nil && declaration.ValueSource.Kind == objectmodel.ValueSourceBinding {
			basis = "binding-declaration:" + string(commit) + ":" + string(declaration.DeclarationDigest)
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

// UnitFailureClaim names an external unit whose value could not be read, so
// a result keeps "failed" apart from "missing" (discovery#U1).
func UnitFailureClaim(failure objectmodel.UnitReadFailure) string {
	return "observation failed for " + objectmodel.AddressLabel(failure.Address) + ": " + string(failure.Code) + ": " + failure.Message
}
