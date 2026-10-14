package server

import (
	inv "kc/invocation"
	"kc/invoke"

	"net/http"

	kcclient "kc/client"
	apphome "kc/home"
	"kc/kernel"
)

// ManagedRepositorySummary is the user's inventory and creation result. The
// durable allocation command stays inside Home; explicit protocol creation
// continues to return its caller-supplied command receipt.
type ManagedRepositorySummary struct {
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
		RepositoryID: item.RepositoryID, Status: item.Status,
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
	mux.HandleFunc("POST /snapshot/v1/connections", f.namedRepositoryCreate)
	mux.HandleFunc("GET /snapshot/v1/connections", f.myRepositories)
	mux.HandleFunc("GET /snapshot/v1/connections/{repository}", f.ownedRepository)
}

func (f *httpFacade) namedRepositoryCreate(w http.ResponseWriter, r *http.Request) {
	var input kcclient.NamedRepositoryCreateRequest
	if !decodeServiceRequest(w, r, &input) {
		return
	}
	if input.Name == "" {
		invoke.WriteJSON(w, http.StatusBadRequest, kernel.FaultJSON(kernel.Fail(kernel.ErrUsageInvalid, "repository creation requires a name")))
		return
	}
	f.executeTyped(w, r, "named-repository-create", "repositories.create", command{Stage: invoke.StageGoverned, Run: VerbCreateNamedRepository}, compactFlags(map[string]FlagValue{"name": input.Name, "store": input.Store}))
}

func VerbCreateNamedRepository(cx *inv.Invocation) (any, error) {
	if cx.WS == nil || cx.WS.Deployment == nil {
		return nil, kernel.Fail(kernel.ErrPreconditionFailed, "managed creation requires a declared deployment")
	}
	item, err := cx.WS.CreateNamedManagedRepository(apphome.ManagedRepositoryRequest{
		Principal: cx.Flag("as"), Name: cx.Flag("name"), Store: cx.Flag("store"),
		IdentityProvider: cx.Flag("_identity-provider"), IdentityIssuer: cx.Flag("_identity-issuer"), IdentitySubject: cx.Flag("_identity-subject"),
	}, func(grant apphome.ManagedRepositoryGrant) error {
		return inv.EnsureManagedRepositoryGrant(cx.Home, grant)
	})
	if err != nil {
		return nil, err
	}
	return summarizeManagedRepository(item), nil
}

// Inventory is scoped to the verified principal and then filtered by current
// metadata permissions. Finding one's allocations grants no Registry discovery,
// Object read, write or share rights.
func (f *httpFacade) myRepositories(w http.ResponseWriter, r *http.Request) {
	f.executeTyped(w, r, "managed-repository-list", "identity.read", command{Stage: invoke.StageGoverned, Run: func(cx *inv.Invocation) (any, error) {
		items, err := cx.WS.ListManagedRepositories(cx.Flag("as"))
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
	f.executeTyped(w, r, "managed-repository-show", "identity.read", command{Stage: invoke.StageGoverned, Run: func(cx *inv.Invocation) (any, error) {
		item, err := cx.WS.GetOwnedManagedRepository(cx.Flag("as"), cx.Flag("repo"))
		if err != nil {
			return nil, err
		}
		if err := authorizeRepositoryMetadata(cx, item); err != nil {
			return nil, err
		}
		return ManagedRepositoryDetail{ManagedRepositorySummary: summarizeManagedRepository(item), Readiness: describeRepositoryReadiness(cx, item)}, nil
	}}, map[string]FlagValue{"repo": r.PathValue("repository")})
}

func authorizeRepositoryMetadata(cx *inv.Invocation, item apphome.ManagedRepositoryResult) error {
	return inv.Authorize(cx.Home, "repository.metadata.read", map[string]FlagValue{"as": cx.Flag("as"), "repo": item.RepositoryID}, cx.Observation.Authorization)
}
