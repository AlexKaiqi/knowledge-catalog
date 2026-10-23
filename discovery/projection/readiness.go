package projection

import (
	retrieval "kc/discovery"
	"kc/kernel"
	objectmodel "kc/object"
)

// IndexReadiness answers "can this Dataset revision be searched right now".
// It is a narrow answer on purpose: BUILDING, UPDATING, FAILED, RETIRED and
// "never built" all collapse into Ready=false, and no internal state name
// reaches the caller. Diagnosis is a separate question (see IndexDiagnosis).
type IndexReadiness struct {
	Dataset     string `json:"dataset"`
	Revision    int    `json:"revision"`
	Ready       bool   `json:"ready"`
	ObjectCount int    `json:"objectCount,omitempty"`
}

// IndexCatchUp is one revision build this instance is currently working on.
// Done and Total count objects, including objects a declaration excludes and
// which therefore produce no searchable document. ObjectCount counts documents,
// so a finished build has Done == Total while ObjectCount is usually smaller;
// the two are not meant to match.
//
// Dataset and Revision are the coordinate the build belongs to, so a caller can
// attribute a progress row to the revision it published. Without them the row
// names only a derived corpus identity, and two revisions of one Dataset would
// be indistinguishable to anyone watching the build they just triggered.
type IndexCatchUp struct {
	Dataset    string              `json:"dataset,omitempty"`
	Revision   int                 `json:"revision,omitempty"`
	Repository kernel.RepositoryID `json:"repository"`
	Done       int                 `json:"done"`
	Total      int                 `json:"total,omitempty"`
}

// IndexDiagnosis explains a readiness result for operators. It is deliberately
// not part of IndexReadiness: computing it costs a full member reconciliation,
// which the common "can I search" question must not pay for.
type IndexDiagnosis struct {
	Dataset  string             `json:"dataset"`
	Revision int                `json:"revision"`
	Ready    bool               `json:"ready"`
	Members  []IndexMemberState `json:"members,omitempty"`
	// Failure names why the last build attempt stopped, in user words.
	Failure string `json:"failure,omitempty"`
	// Basis carries the compiled corpus this revision froze. Diagnosis without
	// it could say "not ready" but not what was compiled, and the two answers
	// come from the same metadata read, so they belong in one response.
	Basis *IndexBasisFacts `json:"basis,omitempty"`
}

// IndexBasisFacts describes the corpus a revision froze. It deliberately has
// no provider repository coordinate: the Dataset pin is the authority basis.
type IndexBasisFacts struct {
	BasisCommit      kernel.CommitID         `json:"basisCommit,omitempty"`
	ObjectCount      int                     `json:"objectCount"`
	AccessDigest     kernel.Digest           `json:"accessDigest,omitempty"`
	PhysicalDigest   kernel.Digest           `json:"physicalDigest,omitempty"`
	ProviderRevision string                  `json:"providerRevision,omitempty"`
	Generation       string                  `json:"generation,omitempty"`
	Coverage         float64                 `json:"coverage,omitempty"`
	Mode             string                  `json:"mode,omitempty"`
	Cause            string                  `json:"cause,omitempty"`
	Schemas          []objectmodel.ObjectID  `json:"schemas,omitempty"`
	Lanes            []string                `json:"lanes,omitempty"`
	Fields           []retrieval.AccessField `json:"fields,omitempty"`
}

// IndexMemberState reports one selected declaration member. Basis is the
// source commit the member was built from; Stage is a user-facing word rather
// than the provider's internal state name.
type IndexMemberState struct {
	Repository  kernel.RepositoryID `json:"repository"`
	BasisCommit kernel.CommitID     `json:"basisCommit"`
	Stage       string              `json:"stage"`
	ObjectCount int                 `json:"objectCount,omitempty"`
}

// User-facing member stages. These are the only words a caller sees.
const (
	IndexStageReady       = "ready"
	IndexStageBuilding    = "building"
	IndexStageFailed      = "failed"
	IndexStageNotBuilt    = "not-built"
	IndexStageUnsupported = "unsupported"
)
