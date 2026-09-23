package cli

import (
	"io"
	"net/url"
	"os"
	"strings"

	"kc/kernel"
)

func repositoryIDFromConnectionURL(raw string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return "", kernel.Fail(kernel.ErrUsageInvalid, "create --url requires an absolute repository URL")
	}
	name := strings.Trim(strings.TrimSuffix(parsed.Path, ".git"), "/")
	if name == "" {
		return "", kernel.Fail(kernel.ErrUsageInvalid, "create --url must identify a repository")
	}
	id := "kr://" + strings.ToLower(parsed.Hostname()) + "/" + name
	if normalized, err := NormalizeCatalogID(id); err != nil || normalized != id {
		return "", kernel.Fail(kernel.ErrUsageInvalid, "create --url cannot derive a valid Repository identity")
	}
	return id, nil
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
