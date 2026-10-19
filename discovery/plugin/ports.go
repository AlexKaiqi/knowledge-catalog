// Package plugin owns the typed Discovery index contribution and its fixed
// Repository selection. Providers do not depend on application executors.
package plugin

import (
	"context"
	retrieval "kc/discovery"
	index "kc/discovery/projection"
	"kc/kernel"
	objectmodel "kc/object"
)

type SearchProjection interface {
	RequiresState(objectmodel.Repository, kernel.CommitID, retrieval.SearchRequest) (bool, error)
	StateView(kernel.RepositoryID, kernel.CommitID) (string, bool)
	SearchAtContext(context.Context, objectmodel.Repository, kernel.CommitID, retrieval.SearchRequest) (retrieval.SearchResult, error)
	SearchStateAtRevisionContext(context.Context, objectmodel.Repository, kernel.CommitID, string, retrieval.SearchRequest) (retrieval.SearchResult, error)
}

type RelationProjection interface {
	RelationsAtContext(context.Context, objectmodel.Repository, kernel.CommitID, retrieval.RelationPageRequest) (retrieval.RelationPage, error)
}

type ProjectionDescriber interface {
	DescribeAt(objectmodel.Repository, kernel.CommitID) (index.IndexDescriptor, error)
}

type ProjectionSynchronizer interface {
	EnsureAt(context.Context, objectmodel.Repository, kernel.CommitID) (index.IndexSync, error)
	Ensure(context.Context, objectmodel.Repository, kernel.CommitID) (index.IndexSync, error)
	RefreshState(context.Context, objectmodel.Repository, kernel.CommitID, index.ObservationSource) (index.StateSync, error)
}

type ProjectionController interface {
	Desire(kernel.RepositoryID, kernel.CommitID) error
	CatchUp(context.Context) error
	Notify(index.ChangeNotice) error
}

type ProjectionStateView interface {
	StateView(kernel.RepositoryID, kernel.CommitID) (string, bool)
}

// SemanticWindowProjection is an optional contribution capability.
type SemanticWindowProjection interface {
	SemanticWindowAtContext(ctx context.Context, repo objectmodel.Repository, commit kernel.CommitID, request retrieval.SearchRequest, queryVector []float32) (retrieval.SearchResult, error)
}
