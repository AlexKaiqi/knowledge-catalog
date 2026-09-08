package cli

import (
	"net/http"

	kcclient "kc/client"
	apphome "kc/home"
	"kc/kernel"
)

func (f *httpFacade) registerConnectionRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /catalog/v1/catalogs/{catalog}/repositories:connect", f.repositoryConnect)
	mux.HandleFunc("GET /catalog/v1/repositories/{repository}/connection", f.repositoryConnectionShow)
	mux.HandleFunc("POST /catalog/v1/repositories/{repository}/connection:check", f.repositoryConnectionCheck)
	mux.HandleFunc("POST /catalog/v1/repositories/{repository}/connection:rotate", f.repositoryConnectionRotate)
}

func (f *httpFacade) repositoryConnect(w http.ResponseWriter, r *http.Request) {
	var q kcclient.ConnectionRequest
	if !decodeServiceRequest(w, r, &q) {
		return
	}
	// Keep secrets in this lexical request scope. Flags feed authorization,
	// telemetry and hooks and must contain only the public coordinates.
	f.executeTyped(w, r, "catalog-repo-connect", "catalog.repositories.connect", command{stage: stageGoverned, run: func(cx *invocation) (any, error) {
		return cx.WS.ConnectRepository(apphome.ConnectionRequest{Catalog: cx.flag("catalog"), Repository: q.Repository, Driver: q.Driver, URL: q.URL, Credential: q.Credential, Principal: cx.flag("as")}, func(g apphome.RepositoryInitialGrant) error { return ensureManagedRepositoryGrant(cx.Home, g) })
	}}, map[string]FlagValue{"catalog": r.PathValue("catalog"), "repo": q.Repository})
}
func (f *httpFacade) repositoryConnectionShow(w http.ResponseWriter, r *http.Request) {
	f.executeTyped(w, r, "catalog-repo-connection-show", "repository.connections.manage", command{stage: stageGoverned, run: func(cx *invocation) (any, error) { return cx.WS.GetConnection(cx.flag("as"), cx.flag("repo")) }}, map[string]FlagValue{"repo": r.PathValue("repository")})
}
func (f *httpFacade) repositoryConnectionCheck(w http.ResponseWriter, r *http.Request) {
	var q struct{}
	if !decodeServiceRequest(w, r, &q) {
		return
	}
	f.executeTyped(w, r, "catalog-repo-connection-check", "repository.connections.manage", command{stage: stageGoverned, run: func(cx *invocation) (any, error) { return cx.WS.CheckConnection(cx.flag("as"), cx.flag("repo")) }}, map[string]FlagValue{"repo": r.PathValue("repository")})
}
func (f *httpFacade) repositoryConnectionRotate(w http.ResponseWriter, r *http.Request) {
	var q kcclient.ConnectionRotationRequest
	if !decodeServiceRequest(w, r, &q) {
		return
	}
	f.executeTyped(w, r, "catalog-repo-connection-rotate", "repository.connections.manage", command{stage: stageGoverned, run: func(cx *invocation) (any, error) {
		return cx.WS.RotateConnectionCredential(cx.flag("as"), cx.flag("repo"), q.Credential)
	}}, map[string]FlagValue{"repo": r.PathValue("repository")})
}

func connectionVerbs() map[string]command {
	// Public clients always use the typed handlers above; no credential enters
	// the generic application flag transport or a local workspace invocation.
	remoteOnly := command{stage: stageGoverned, run: func(*invocation) (any, error) {
		return nil, kernel.Fail(kernel.ErrUsageInvalid, "connections require the typed Server API")
	}}
	return map[string]command{"catalog-repo-connect": remoteOnly, "catalog-repo-connection-show": remoteOnly, "catalog-repo-connection-check": remoteOnly, "catalog-repo-connection-rotate": remoteOnly}
}
