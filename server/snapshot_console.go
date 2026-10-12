package server

import (
	"kc/home"
	"kc/identity"
	"kc/kernel"
	"kc/snapshot"
	"strings"
)

// A console link identifies a source in the already authenticated lakeFS
// instance. It conveys no credential or permission, and never guesses the
// native repository name from a logical kr:// identity.
func snapshotConsoleRepository(bindings []home.HomeRepo, caller identity.HTTPIdentity, id kernel.RepositoryID) string {
	if caller.User == nil || caller.User.Provider != "lakefs" {
		return ""
	}
	for _, binding := range bindings {
		if binding.ID != string(id) || binding.Driver != "lakefs" {
			continue
		}
		endpoint, err := snapshot.ParseLakeFSDSN(binding.DSN)
		if err == nil && endpoint.Origin == strings.TrimRight(caller.User.Issuer, "/") {
			return endpoint.Repository
		}
	}
	return ""
}
