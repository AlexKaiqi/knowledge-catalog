package cli

import (
	"io"
	"kc/snapshot"
	"os"
	"strings"

	"kc/kernel"
)

func repositoryIDFromConnectionURL(raw string) (string, error) {
	endpoint, err := snapshot.ParseLakeFSDSN(strings.TrimSpace(raw))
	if err != nil {
		return "", err
	}
	if !snapshot.ValidLakeFSName(endpoint.Repository) || snapshot.PlatformLakeFSName(endpoint.Repository) {
		return "", kernel.Fail(kernel.ErrUsageInvalid, "create --url must identify a business Repository")
	}
	return endpoint.Repository, nil
}

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
