package cli

import (
	"net/http"

	kcclient "kc/client"
	apphome "kc/home"
	"kc/kernel"
)

// ManagedRepositorySummary is the user's inventory and creation result. The
// durable allocation command stays inside Home; explicit protocol creation
// continues to return its caller-supplied command receipt.
type ManagedRepositorySummary struct {
	Catalog           string          `json:"catalog"`
	RepositoryID      string          `json:"repositoryId"`
	Status            string          `json:"status"`
	Head              kernel.CommitID `json:"head"`
	Name              string          `json:"name,omitempty"`
	Owner             string          `json:"owner"`
	Store             string          `json:"store"`
	ManagementURL     string          `json:"managementURL"`
	ManagementState   string          `json:"managementState,omitempty"`
	ProviderURL       string          `json:"providerURL,omitempty"`
	ProvisioningState string          `json:"provisioningState"`
}

func summarizeManagedRepository(item apphome.ManagedRepositoryResult) ManagedRepositorySummary {
	return ManagedRepositorySummary{
		Catalog: item.Catalog, RepositoryID: item.RepositoryID, Status: item.Status,
		Head: item.Head, Name: item.Name, Owner: item.Owner, Store: item.Store,
		ManagementURL: item.ManagementURL, ManagementState: item.ManagementState,
		ProviderURL: item.ProviderURL, ProvisioningState: item.ProvisioningState,
	}
}

func (f *httpFacade) registerRepositoryRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /repositories/{repository}", f.repositoryPage)
	mux.HandleFunc("GET /assets/repository.js", f.repositoryScript)
	mux.HandleFunc("GET /console", f.consolePage)
	mux.HandleFunc("GET /assets/console.js", f.consoleScript)
	mux.HandleFunc("POST /catalog/v1/repositories", f.namedRepositoryCreate)
	mux.HandleFunc("GET /catalog/v1/repositories", f.myRepositories)
	mux.HandleFunc("GET /catalog/v1/repositories/{repository}", f.ownedRepository)
}

func (f *httpFacade) namedRepositoryCreate(w http.ResponseWriter, r *http.Request) {
	var input kcclient.NamedRepositoryCreateRequest
	if !decodeServiceRequest(w, r, &input) {
		return
	}
	if input.Name == "" {
		writeJSON(w, http.StatusBadRequest, kernel.FaultJSON(kernel.Fail(kernel.ErrUsageInvalid, "repository creation requires a name")))
		return
	}
	f.executeTyped(w, r, "named-repository-create", "catalog.repositories.create", command{stage: stageGoverned, run: verbCreateNamedRepository}, compactFlags(map[string]FlagValue{"name": input.Name, "store": input.Store, "catalog": input.Catalog}))
}

func verbCreateNamedRepository(cx *invocation) (any, error) {
	if cx.WS == nil || cx.WS.Deployment == nil {
		return nil, kernel.Fail(kernel.ErrPreconditionFailed, "managed creation requires a declared deployment")
	}
	catalogID := cx.flag("catalog")
	if catalogID == "" && len(cx.WS.File.Catalogs) > 0 {
		catalogID = cx.WS.File.Catalogs[0].ID
	}
	item, err := cx.WS.CreateNamedManagedRepository(apphome.ManagedRepositoryRequest{
		CatalogID: catalogID, Principal: cx.flag("as"), Name: cx.flag("name"), Store: cx.flag("store"),
		IdentityProvider: cx.flag("_identity-provider"), IdentityIssuer: cx.flag("_identity-issuer"), IdentitySubject: cx.flag("_identity-subject"),
	}, func(grant apphome.ManagedRepositoryGrant) error { return ensureManagedRepositoryGrant(cx.Home, grant) })
	if err != nil {
		return nil, err
	}
	return summarizeManagedRepository(item), nil
}

// Inventory is scoped to the verified principal and then filtered by current
// metadata permissions. Finding one's allocations grants no Catalog discovery,
// Knowledge read, write or share rights.
func (f *httpFacade) myRepositories(w http.ResponseWriter, r *http.Request) {
	f.executeTyped(w, r, "managed-repository-list", "identity.read", command{stage: stageGoverned, run: func(cx *invocation) (any, error) {
		items, err := cx.WS.ListManagedRepositories(cx.flag("as"))
		if err != nil {
			return nil, err
		}
		allowed := make([]ManagedRepositorySummary, 0, len(items))
		for _, item := range items {
			if err := authorizeRepositoryMetadata(cx, item); err == nil {
				allowed = append(allowed, summarizeManagedRepository(item))
			} else if kernel.CodeOf(err) != kernel.ErrForbidden {
				return nil, err
			}
		}
		return map[string]any{"repositories": allowed}, nil
	}}, map[string]FlagValue{})
}

func (f *httpFacade) ownedRepository(w http.ResponseWriter, r *http.Request) {
	f.executeTyped(w, r, "managed-repository-show", "identity.read", command{stage: stageGoverned, run: func(cx *invocation) (any, error) {
		item, err := cx.WS.GetOwnedManagedRepository(cx.flag("as"), cx.flag("repo"))
		if err != nil {
			return nil, err
		}
		if err := authorizeRepositoryMetadata(cx, item); err != nil {
			return nil, err
		}
		return ManagedRepositoryDetail{ManagedRepositorySummary: summarizeManagedRepository(item), Readiness: describeRepositoryReadiness(cx, item)}, nil
	}}, map[string]FlagValue{"repo": r.PathValue("repository")})
}

func authorizeRepositoryMetadata(cx *invocation, item apphome.ManagedRepositoryResult) error {
	return authorize(cx.Home, "repository.metadata.read", map[string]FlagValue{"as": cx.flag("as"), "repo": item.RepositoryID, "catalog": item.Catalog}, cx.Observation.authorization)
}
