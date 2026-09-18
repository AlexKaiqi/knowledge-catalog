package lakefs

import (
	"net/http"
	"os"
	"strings"

	"kc/kernel"
)

func managedStorageNamespace(prefix, gravelerName string) string {
	return strings.TrimRight(strings.TrimSpace(prefix), "/") + "/" + gravelerName
}

type repoInfo struct {
	ID               string `json:"id"`
	DefaultBranch    string `json:"default_branch"`
	StorageNamespace string `json:"storage_namespace"`
}

func verifyManaged(info repoInfo, gravelerName, storagePrefix string) error {
	want := managedStorageNamespace(storagePrefix, gravelerName)
	if info.ID != gravelerName || info.StorageNamespace != want {
		return kernel.Fail(kernel.ErrPreconditionFailed, "LakeFS repository is not the expected managed allocation")
	}
	return nil
}

// CreateManaged creates or resumes only the Graveler repository named by the
// DSN. The DSN's final path segment is the visible lakeFS id (--name).
// Storage namespace is {prefix}/{name}. Allocation is the ledger token only.
// kc- is reserved for platform repositories.
func CreateManaged(id kernel.RepositoryID, dsn, credential, allocation, storagePrefix string) (*Repository, string, error) {
	if strings.TrimSpace(allocation) == "" || strings.TrimSpace(storagePrefix) == "" {
		return nil, "", kernel.Fail(kernel.ErrUsageInvalid, "managed allocation identity and storage namespace are required")
	}
	ep, err := ParseDSN(dsn)
	if err != nil {
		return nil, "", err
	}
	if !ValidGravelerName(ep.Repository) {
		return nil, "", kernel.Fail(kernel.ErrUsageInvalid, "managed LakeFS DSN must name a Graveler repository")
	}
	if credential == "" {
		credential = os.Getenv(EnvCredential)
	}
	client, err := newAPIClient(ep, credential)
	if err != nil {
		return nil, "", err
	}
	var info repoInfo
	status, _, probeErr := client.doJSON(http.MethodGet, client.repositoryPath(""), nil, &info)
	if status == http.StatusNotFound {
		create := map[string]string{
			"name":              ep.Repository,
			"storage_namespace": managedStorageNamespace(storagePrefix, ep.Repository),
			"default_branch":    defaultBranch,
		}
		if _, _, err := client.doJSON(http.MethodPost, "/repositories", create, &info); err != nil {
			if _, _, readErr := client.doJSON(http.MethodGet, client.repositoryPath(""), nil, &info); readErr != nil {
				return nil, "", err
			}
		}
	} else if probeErr != nil {
		return nil, "", probeErr
	}
	if err := verifyManaged(info, ep.Repository, storagePrefix); err != nil {
		return nil, "", err
	}
	repo, err := OpenExisting(id, dsn, credential)
	if err != nil {
		return nil, "", err
	}
	return repo, info.StorageNamespace, nil
}

// OpenManaged verifies the DSN names the live Graveler repository and that
// the durable storage namespace still matches. Missing or replaced
// repositories are never recreated.
func OpenManaged(id kernel.RepositoryID, dsn, credential, allocation, backendID string) (*Repository, error) {
	if strings.TrimSpace(allocation) == "" {
		return nil, kernel.Fail(kernel.ErrPreconditionFailed, "managed LakeFS allocation is missing")
	}
	if credential == "" {
		credential = os.Getenv(EnvCredential)
	}
	ep, err := ParseDSN(dsn)
	if err != nil {
		return nil, err
	}
	client, err := newAPIClient(ep, credential)
	if err != nil {
		return nil, err
	}
	var info repoInfo
	if _, _, err := client.doJSON(http.MethodGet, client.repositoryPath(""), nil, &info); err != nil {
		return nil, err
	}
	if info.ID != ep.Repository {
		return nil, kernel.Fail(kernel.ErrPreconditionFailed, "LakeFS repository is not the expected managed allocation")
	}
	if backendID != "" && info.StorageNamespace != backendID {
		return nil, kernel.Fail(kernel.ErrPreconditionFailed, "managed LakeFS backend identity changed")
	}
	return OpenExisting(id, dsn, credential)
}
