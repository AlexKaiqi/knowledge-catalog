package invocation

import (
	retrieval "kc/discovery"
	"kc/kernel"
)

func VerbPluginCapabilities(cx *Invocation) (any, error) {
	return cx.WS.Plugins.CapabilityManifests(cx.Flag("name"))
}

func VerbTrialDeclarations(cx *Invocation) (any, error) {
	request, ok := cx.Flags["_declaration-trial"].(retrieval.DeclarationTrialRequest)
	if !ok {
		return nil, kernel.Fail(kernel.ErrUsageInvalid, "missing declaration trial documents")
	}
	return cx.WS.TrialIndexDeclarations(cx.Flag("name"), request, func(id kernel.RepositoryID) error {
		if !AllowedRepoRead(cx.Home, cx.Flags, string(id), "") {
			return kernel.Fail(kernel.ErrForbidden, "trial requires complete object.read access to each referenced Schema repository")
		}
		return nil
	})
}
