// Package lakefs is the public registration shim for the LakeFS snapshot
// adapter (CAP-ORG-01 B10, ruling C): the implementation lives in
// snapshot/internal/lakefsimpl and only this shim may import it, so callers
// keep a stable, provider-neutral-looking path while the concrete adapter
// stays behind the capability boundary.
package lakefs

import "kc/snapshot/internal/lakefsimpl"

const EnvCredential = lakefsimpl.EnvCredential

type (
	// Endpoint is the parsed lakefs DSN.
	Endpoint = lakefsimpl.Endpoint
	// Repository is the lakefs-backed snapshot store.
	Repository = lakefsimpl.Repository
)

var (
	CreateManaged        = lakefsimpl.CreateManaged
	GravelerSlug         = lakefsimpl.GravelerSlug
	ManagedGravelerName  = lakefsimpl.ManagedGravelerName
	OpenExisting         = lakefsimpl.OpenExisting
	OpenManaged          = lakefsimpl.OpenManaged
	ParseDSN             = lakefsimpl.ParseDSN
	PlatformGravelerName = lakefsimpl.PlatformGravelerName
	ReadStamp            = lakefsimpl.ReadStamp
	ValidGravelerName    = lakefsimpl.ValidGravelerName
	WriteStamp           = lakefsimpl.WriteStamp
)
