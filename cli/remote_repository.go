package cli

import (
	"context"

	kcclient "kc/client"
	"kc/kernel"
)

func runRemoteRegistry(ctx context.Context, client *kcclient.Client, path string, flags map[string]FlagValue, options kcclient.RequestOptions) (any, error) {
	service := client.SnapshotService()
	if path == "create" {
		if err := validateManagedRepositoryCreateFlags(flags); err != nil {
			return nil, err
		}
		var output any
		var err error
		if flagString(flags, "name") != "" {
			err = service.CreateNamedRepository(ctx, kcclient.NamedRepositoryCreateRequest{
				Name: flagString(flags, "name"), Store: flagString(flags, "store"),
			}, options, &output)
		} else {
			repository, deriveErr := repositoryIDFromConnectionURL(flagString(flags, "url"))
			if deriveErr != nil {
				return nil, deriveErr
			}
			credential, credentialErr := connectionCredentialFile(flags)
			if credentialErr != nil {
				return nil, credentialErr
			}
			err = service.ConnectRepository(ctx, kcclient.ConnectionRequest{
				Repository: repository, Driver: "lakefs", URL: flagString(flags, "url"), Credential: credential,
			}, options, &output)
		}
		return output, err
	}
	var err error
	var output any
	switch path {
	case "show":
		err = service.Show(ctx, options, &output)
	case "registry audit":
		var limit int
		limit, err = remoteLimit(flags)
		if err == nil {
			err = service.Audit(ctx, limit, flagString(flags, "continuation"), options, &output)
		}
	case "attach":
		err = service.AttachRepository(ctx, kcclient.RepositoryAttachRequest{Repository: flagString(flags, "repo")}, options, &output)
	case "detach":
		err = service.DetachRepository(ctx, flagString(flags, "repo"), options, &output)
	default:
		return nil, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "remote typed client does not implement %s", path)
	}
	return output, err
}
