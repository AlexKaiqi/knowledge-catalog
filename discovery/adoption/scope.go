// Package adoption owns the shared facts of compiled plugin adoption.
// It interprets source configuration without constructing capability runtimes.
package adoption

import "kc/kernel"

// PluginScope is supplied by assembly from Repository configuration. Own
// records acquired resources immediately, including resources of a failed open.
type PluginScope struct {
	Name         string
	Instance     string
	Repositories []kernel.RepositoryID
	Own          func(func() error)
}
