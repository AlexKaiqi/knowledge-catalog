// Package gitea is the public registration shim for the Gitea snapshot
// adapter (CAP-ORG-01 B10, ruling C): the implementation lives in
// snapshot/internal/giteaimpl and only this shim may import it, so callers
// keep a stable path while the concrete adapter stays behind the capability
// boundary.
package gitea

import "kc/snapshot/internal/giteaimpl"

const EnvToken = giteaimpl.EnvToken

type (
	// Endpoint is the parsed gitea DSN.
	Endpoint = giteaimpl.Endpoint
	// ManagedUserRequest asks for one service-managed identity.
	ManagedUserRequest = giteaimpl.ManagedUserRequest
	// Repository is the gitea-backed snapshot store.
	Repository = giteaimpl.Repository
)

var (
	CreateManaged        = giteaimpl.CreateManaged
	CreateManagedForUser = giteaimpl.CreateManagedForUser
	EnsureManagedUser    = giteaimpl.EnsureManagedUser
	Open                 = giteaimpl.Open
	OpenConnection       = giteaimpl.OpenConnection
	OpenExisting         = giteaimpl.OpenExisting
	OpenManaged          = giteaimpl.OpenManaged
	ParseDSN             = giteaimpl.ParseDSN
	ReadStamp            = giteaimpl.ReadStamp
	WriteStamp           = giteaimpl.WriteStamp
)
