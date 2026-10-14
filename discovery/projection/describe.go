package projection

import (
	retrieval "kc/discovery"
	"kc/kernel"
	objectmodel "kc/object"
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
	Schemas          []objectmodel.ObjectID  `json:"schemas,omitempty"`
	Lanes            []string                `json:"lanes,omitempty"`
	Fields           []retrieval.AccessField `json:"fields,omitempty"`
}
