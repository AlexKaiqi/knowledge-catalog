package invocation

import (
	"kc/invoke"
	"kc/kernel"
	objectmodel "kc/object"
)

// History crosses the delivered commit boundary. Authorize it as a source
// repository operation, without Dataset consumption scopes or their defaults.
func authorizeRepositoryHistory(cx *Invocation, repository kernel.RepositoryID, object objectmodel.ObjectID) error {
	flags := map[string]invoke.FlagValue{
		"as": cx.Flag("as"), "repo": string(repository), "object": string(object), "ref": cx.Flag("ref"),
	}
	return Authorize(cx.Home, "object.history.read", flags, nil, cx.WS)
}
