package projection

import (
	"context"
	"kc/kernel"
	objectmodel "kc/object"
)

// StateSync reports one refresh of the dynamic State projection. The
// declaration basis remains a Repository commit; Revision identifies the
// provider generation published for the accompanying observations.
type StateSync struct {
	Repository        kernel.RepositoryID `json:"repository"`
	BasisCommit       kernel.CommitID     `json:"basisCommit"`
	Revision          string              `json:"revision"`
	AccessDigest      kernel.Digest       `json:"accessDigest"`
	ObservationDigest kernel.Digest       `json:"observationDigest"`
	ObjectCount       int                 `json:"objectCount"`
	Updated           int                 `json:"updated"`
	Removed           int                 `json:"removed"`
	Mode              string              `json:"mode"`
}

// ObservationSource supplies verified logical values on fixed declaration coordinates.
// Projection has no source protocol, credentials or HTTP dependency.
type ObservationSource interface {
	ObserveRepositoryValue(context.Context, objectmodel.Repository, objectmodel.KnowledgeValue) (objectmodel.ObservedValue, error)
}
