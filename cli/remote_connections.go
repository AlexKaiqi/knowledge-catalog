package cli

import (
	"context"
	"io"
	"os"
	"strings"

	kcclient "kc/client"
	apphome "kc/home"
	"kc/kernel"
)

func connectionCredentialFile(flags map[string]FlagValue) (string, error) {
	name, err := requireRemoteFlag(flags, "credential-file")
	if err != nil {
		return "", err
	}
	file, err := os.Open(name)
	if err != nil {
		return "", kernel.Fail(kernel.ErrUsageInvalid, "cannot read credential file")
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() {
		return "", kernel.Fail(kernel.ErrUsageInvalid, "credential file must be a regular file")
	}
	raw, err := io.ReadAll(io.LimitReader(file, 65537))
	if err != nil || len(raw) > 65536 {
		return "", kernel.Fail(kernel.ErrUsageInvalid, "credential file cannot exceed 64 KiB")
	}
	credential := strings.TrimSpace(string(raw))
	if credential == "" || strings.ContainsAny(credential, "\r\n") {
		return "", kernel.Fail(kernel.ErrUsageInvalid, "credential file must contain one nonempty credential")
	}
	return credential, nil
}

func runRemoteConnections(ctx context.Context, client *kcclient.Client, path string, flags map[string]FlagValue, o kcclient.RequestOptions) (any, error) {
	allowed := flagNames("repo server as request-id trace-id span-id parent-span-id help")
	if path == "catalog repo connect" {
		for _, name := range []string{"catalog", "driver", "url", "credential-file"} {
			allowed[name] = struct{}{}
		}
	} else if path == "catalog repo connection rotate" {
		allowed["credential-file"] = struct{}{}
	}
	if err := rejectFlagsOutside(flags, allowed, "kc "+path); err != nil {
		return nil, err
	}
	repository, err := requireRemoteFlag(flags, "repo")
	if err != nil {
		return nil, err
	}
	service := client.CatalogService()
	var result apphome.ConnectionResult
	switch path {
	case "catalog repo connect":
		catalog, err := requireRemoteFlag(flags, "catalog")
		if err != nil {
			return nil, err
		}
		url, err := requireRemoteFlag(flags, "url")
		if err != nil {
			return nil, err
		}
		credential, err := connectionCredentialFile(flags)
		if err != nil {
			return nil, err
		}
		driver := FlagString(flags, "driver")
		if driver == "" {
			driver = "gitea"
		}
		err = service.ConnectRepository(ctx, catalog, kcclient.ConnectionRequest{Repository: repository, Driver: driver, URL: url, Credential: credential}, o, &result)
		return result, err
	case "catalog repo connection show":
		err = service.RepositoryConnection(ctx, repository, o, &result)
		return result, err
	case "catalog repo connection check":
		err = service.CheckRepositoryConnection(ctx, repository, o, &result)
		return result, err
	case "catalog repo connection rotate":
		credential, err := connectionCredentialFile(flags)
		if err != nil {
			return nil, err
		}
		err = service.RotateRepositoryConnection(ctx, repository, kcclient.ConnectionRotationRequest{Credential: credential}, o, &result)
		return result, err
	}
	return nil, kernel.Fail(kernel.ErrUsageInvalid, "unknown connection command")
}
