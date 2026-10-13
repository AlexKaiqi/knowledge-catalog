package server

import (
	"context"
	"strings"

	"kc/home"
	"kc/identity"
	inv "kc/invocation"
	"kc/kernel"
	"kc/snapshot/access"
)

func (f *httpFacade) nativeRepositoryCreator(opened *Home) func(context.Context, identity.HTTPIdentity, string) (kernel.RepositoryID, string, error) {
	if opened.Deployment == nil {
		return nil
	}
	return func(ctx context.Context, caller identity.HTTPIdentity, name string) (kernel.RepositoryID, string, error) {
		if err := nativeCreationPoolMatches(opened.Deployment, f.options.NativeAPIOrigin); err != nil {
			return "", "", err
		}
		flags := snapshotRequestFlags(ctx, caller, access.Query{Action: "repositories.create"})
		if err := inv.Authorize(opened.Dir, "repositories.create", flags, nil, opened); err != nil {
			return "", "", err
		}
		item, err := opened.CreateNamedManagedRepository(home.ManagedRepositoryRequest{Principal: caller.Principal, Name: name, IdentityProvider: caller.User.Provider, IdentityIssuer: caller.User.Issuer, IdentitySubject: caller.User.Subject}, func(grant home.ManagedRepositoryGrant) error {
			return inv.EnsureManagedRepositoryGrant(opened.Dir, grant)
		})
		if err != nil {
			return "", "", err
		}
		for native, id := range snapshotNativeRepositories(opened.BindingFile().Repos, f.options.NativeAPIOrigin) {
			if string(id) == item.RepositoryID {
				return id, native, nil
			}
		}
		return "", "", kernel.Fail(kernel.ErrPreconditionFailed, "created repository has no unique native binding")
	}
}

func nativeCreationPoolMatches(config *home.DeploymentConfig, origin string) error {
	if len(config.ManagedStores) > 1 {
		return kernel.Fail(kernel.ErrPreconditionFailed, "native creation requires one explicitly configured allocation pool")
	}
	var pool *home.ManagedRepositoryConfig
	if config.ManagedRepositories != nil {
		pool = config.ManagedRepositories
	}
	for _, candidate := range config.ManagedStores {
		copy := candidate
		pool = &copy
	}
	if pool == nil {
		return kernel.Fail(kernel.ErrPreconditionFailed, "managed repository pool is not configured")
	}
	endpoint, err := identity.VerifierOrigin(pool.DSN, "lakeFS")
	if err != nil || endpoint != strings.TrimRight(origin, "/") {
		return kernel.Fail(kernel.ErrPreconditionFailed, "managed pool must belong to the authenticated native instance")
	}
	return nil
}
