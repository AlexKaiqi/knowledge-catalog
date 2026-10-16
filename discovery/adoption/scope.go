// Package adoption owns the shared facts of compiled plugin adoption.
// It interprets source configuration without constructing capability runtimes.
package adoption

import "kc/kernel"

// PluginScope is supplied by assembly for the host's admitted sources. Own
// records acquired resources immediately, including resources of a failed open.
type PluginScope struct {
	Name         string
	Instance     string
	Repositories []kernel.RepositoryID
	// ContainsRepository checks the host's current admitted sources. It must
	// be safe for concurrent calls; nil retains the fixed Repositories scope.
	// Membership does not grant caller permissions.
	ContainsRepository func(kernel.RepositoryID) bool
	// RepositoryInventory supplies a current in-memory snapshot for maintenance.
	// Nil retains Repositories; the host owns admission and concurrent safety.
	RepositoryInventory func() []kernel.RepositoryID
	Own                 func(func() error)
}
