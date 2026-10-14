package cli

import (
	"context"

	kcclient "kc/client"
	"kc/invoke"
)

func runRemoteRepositoryCreate(ctx context.Context, client *kcclient.Client, flags map[string]FlagValue, options kcclient.RequestOptions) (any, error) {
	service := client.RepositoryManagementService()
	if err := validateManagedRepositoryCreateFlags(flags); err != nil {
		return nil, err
	}
	var output any
	var err error
	if invoke.FlagString(flags, "name") != "" {
		err = service.CreateNamedRepository(ctx, kcclient.NamedRepositoryCreateRequest{
			Name: invoke.FlagString(flags, "name"), Store: invoke.FlagString(flags, "store"),
		}, options, &output)
	} else {
		repository, deriveErr := repositoryIDFromConnectionURL(invoke.FlagString(flags, "url"))
		if deriveErr != nil {
			return nil, deriveErr
		}
		credential, credentialErr := connectionCredentialFile(flags)
		if credentialErr != nil {
			return nil, credentialErr
		}
		err = service.ConnectRepository(ctx, kcclient.ConnectionRequest{
			Repository: repository, Driver: "lakefs", URL: invoke.FlagString(flags, "url"), Credential: credential,
		}, options, &output)
	}
	return output, err
}
