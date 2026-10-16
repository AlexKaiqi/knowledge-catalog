package server

import (
	"kc/home"
	"kc/identity"
	"kc/kernel"
	"kc/snapshot"
	"strings"
)

// A console link identifies a source in the Server's configured lakeFS
// instance. It conveys no credential or permission, and never guesses the
// native repository name from a logical kr:// identity.
func snapshotConsoleRepository(bindings []home.HomeRepo, caller identity.HTTPIdentity, id kernel.RepositoryID, origin string) string {
	if origin == "" || caller.Principal == "" {
		return ""
	}
	for _, binding := range bindings {
		if binding.ID != string(id) || binding.Driver != "lakefs" {
			continue
		}
		endpoint, err := snapshot.ParseLakeFSDSN(binding.DSN)
		if err == nil && endpoint.Origin == strings.TrimRight(origin, "/") {
			return endpoint.Repository
		}
	}
	return ""
}

func snapshotNativeRepositories(bindings []home.HomeRepo, origin string) map[string]kernel.RepositoryID {
	result := map[string]kernel.RepositoryID{}
	ambiguous := map[string]bool{}
	for _, binding := range bindings {
		if binding.Driver != "lakefs" {
			continue
		}
		endpoint, err := snapshot.ParseLakeFSDSN(binding.DSN)
		if err != nil || endpoint.Origin != strings.TrimRight(origin, "/") {
			continue
		}
		if previous, exists := result[endpoint.Repository]; exists && previous != kernel.RepositoryID(binding.ID) {
			ambiguous[endpoint.Repository] = true
		}
		result[endpoint.Repository] = kernel.RepositoryID(binding.ID)
	}
	for name := range ambiguous {
		delete(result, name)
	}
	return result
}
