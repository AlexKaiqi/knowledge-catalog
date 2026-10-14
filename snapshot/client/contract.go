// Package client provides the independent Snapshot Store HTTP SDK.
package client

import (
	"context"
	"net/http"
)

// Transport reuses the caller's authenticated transport. It owns credentials,
// session refresh and trace propagation; this SDK neither asserts a principal
// nor keeps its own login or authorization state.
type Transport interface {
	Do(context.Context, string, *http.Request) (*http.Response, error)
}

type Config struct {
	BaseURL   string
	Transport Transport
}

// RequestOptions are diagnostic correlation, never proof or authorization.
type RequestOptions struct {
	RequestID string
}

type Client interface {
	SnapshotService() Service
	WorkspaceFilesService() FilesService
	AdminService() AccessService
}

// Service addresses the service's registration space, Dataset publications
// and native repository attributes. Native names are never inferred from IDs.
type Service interface {
	Show(context.Context, RequestOptions, any) error
	Audit(context.Context, int, string, RequestOptions, any) error
	Repositories(context.Context, RequestOptions, any) error
	AttachRepository(context.Context, RepositoryAttachRequest, RequestOptions, any) error
	DetachRepository(context.Context, string, RequestOptions, any) error
	KnowledgeSets(context.Context, RequestOptions, any) error
	DefineKnowledgeSet(context.Context, KnowledgeSetRequest, RequestOptions, any) error
	ResolveDefinition(context.Context, KnowledgeSetRequest, RequestOptions, any) error
	KnowledgeSet(context.Context, string, RequestOptions, any) error
	ShowDataset(context.Context, string, DatasetShowOptions, RequestOptions, any) error
	RetireKnowledgeSet(context.Context, string, RequestOptions, any) error
	ResolveKnowledgeSet(context.Context, string, KnowledgeSetResolveRequest, RequestOptions, any) error
	CheckKnowledgeSet(context.Context, string, KnowledgeSetResolveRequest, RequestOptions, any) error
	RepositoryMetadata(context.Context, string, RequestOptions) (map[string]string, error)
	SetRepositoryMetadata(context.Context, string, map[string]string, RequestOptions) error
	DeleteRepositoryMetadata(context.Context, string, []string, RequestOptions) error
}

type FilesService interface {
	Mounts(context.Context, WorkspaceFileMountsRequest, RequestOptions, any) error
	Directory(context.Context, WorkspaceFileDirectoryRequest, RequestOptions, any) error
	Read(context.Context, WorkspaceFileReadRequest, RequestOptions, any) error
}

type AccessService interface {
	AddGrant(context.Context, GrantRequest, RequestOptions, any) error
	Grants(context.Context, RequestOptions, any) error
	GrantsMatching(context.Context, GrantRequest, RequestOptions, any) error
	UpdateGrant(context.Context, string, GrantUpdateRequest, RequestOptions, any) error
	RemoveGrant(context.Context, string, RequestOptions, any) error
	Admission(context.Context, RequestOptions, any) error
}
