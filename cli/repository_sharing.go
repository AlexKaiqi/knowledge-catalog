package cli

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"slices"
	"sort"

	kcclient "kc/client"
	"kc/identity"
	"kc/kernel"
)

// AdmissionReceipt survives ordinary rule revocation. Repeating admission
// must report its original decision without recreating any removed rule.
type AdmissionReceipt struct {
	Principal string   `json:"principal"`
	Catalog   string   `json:"catalog"`
	Actions   []string `json:"actions"`
}

func (f *httpFacade) registerAdmissionSharingRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /identity/v1/admission", f.admissionShow)
	mux.HandleFunc("GET /catalog/v1/repositories/{repository}/shares", f.repositoryShareList)
	mux.HandleFunc("POST /catalog/v1/repositories/{repository}/shares", f.repositoryShareCreate)
	mux.HandleFunc("DELETE /catalog/v1/repositories/{repository}/shares/{share}", f.repositoryShareRevoke)
}

func (f *httpFacade) admissionShow(w http.ResponseWriter, r *http.Request) {
	f.executeTyped(w, r, "admission-show", "identity.read", command{stage: stageHome, run: func(cx *invocation) (any, error) {
		return admissionOverview(cx)
	}}, map[string]FlagValue{})
}

func admissionOverview(cx *invocation) (kcclient.AdmissionResult, error) {
	out := kcclient.AdmissionResult{
		Principal: cx.flag("as"),
		Grants:    []kcclient.AdmissionGrant{},
		Request:   kcclient.AdmissionRequest{Administrators: []string{}},
	}
	file, err := ReadAllow(cx.Home)
	if err != nil {
		return out, err
	}
	administrators := map[string]bool{}
	for _, rule := range file.Rules {
		if rule.Principal == out.Principal {
			out.Grants = append(out.Grants, kcclient.AdmissionGrant{
				ID: rule.ID, Actions: slices.Clone(rule.Actions), Catalog: rule.Catalog,
				Repository: rule.Repo, SharedBy: rule.SharedBy,
			})
		}
		if administrators[rule.Principal] {
			continue
		}
		for _, action := range rule.Actions {
			if actionMatches(action, "admin.grants.manage") {
				administrators[rule.Principal] = true
				break
			}
		}
	}
	for principal := range administrators {
		out.Request.Administrators = append(out.Request.Administrators, principal)
	}
	sort.Strings(out.Request.Administrators)
	if cx.WS != nil && cx.WS.Deployment != nil && cx.WS.Deployment.Admission != nil {
		out.Request.URL = cx.WS.Deployment.Admission.RequestURL
	}
	return out, nil
}

func (f *httpFacade) repositoryShareList(w http.ResponseWriter, r *http.Request) {
	f.executeTyped(w, r, "catalog-repo-share-list", "repository.shares.manage", command{stage: stageHome, run: verbRepositoryShareList}, map[string]FlagValue{"repo": r.PathValue("repository")})
}

func (f *httpFacade) repositoryShareCreate(w http.ResponseWriter, r *http.Request) {
	var request kcclient.RepositoryShareRequest
	if !decodeServiceRequest(w, r, &request) {
		return
	}
	f.executeTyped(w, r, "catalog-repo-share-add", "repository.shares.manage", command{stage: stageHome, run: func(cx *invocation) (any, error) {
		return createRepositoryShare(cx, request)
	}}, map[string]FlagValue{"repo": r.PathValue("repository")})
}

func (f *httpFacade) repositoryShareRevoke(w http.ResponseWriter, r *http.Request) {
	f.executeTyped(w, r, "catalog-repo-share-remove", "repository.shares.manage", command{stage: stageHome, run: verbRepositoryShareRemove}, map[string]FlagValue{"repo": r.PathValue("repository"), "id": r.PathValue("share")})
}

func sharingPolicy(cx *invocation) ([]string, error) {
	if cx.WS == nil || cx.WS.Deployment == nil {
		return nil, kernel.Fail(kernel.ErrPreconditionFailed, "repository sharing requires a declared deployment")
	}
	return cx.WS.RepositoryShareActions(cx.flag("repo"))
}

// wholeRepositoryActionAllowed deliberately refuses to widen grants limited
// to one Catalog/ref/object/aspect/workspace into a full repository share.
func wholeRepositoryActionAllowed(rules []AllowRule, principal, repository, action string) bool {
	for _, rule := range rules {
		if rule.Principal != principal || (rule.Repo != "" && rule.Repo != repository) || rule.Catalog != "" || rule.Ref != "" || rule.Object != "" || rule.Aspect != "" || rule.Dataset != "" {
			continue
		}
		for _, granted := range rule.Actions {
			if actionMatches(granted, action) {
				return true
			}
		}
	}
	return false
}

func shareRule(rule AllowRule) kcclient.RepositoryShare {
	return kcclient.RepositoryShare{ID: rule.ShareID, Repository: rule.Repo, Principal: rule.Principal, Actions: slices.Clone(rule.Actions), SharedBy: rule.SharedBy}
}

func verbRepositoryShareList(cx *invocation) (any, error) {
	policy, err := sharingPolicy(cx)
	if err != nil {
		return nil, err
	}
	file, err := ReadAllow(cx.Home)
	if err != nil {
		return nil, err
	}
	out := kcclient.RepositoryShares{Repository: cx.flag("repo"), AllowedActions: []string{}, Shares: []kcclient.RepositoryShare{}}
	for _, action := range policy {
		if wholeRepositoryActionAllowed(file.Rules, cx.flag("as"), cx.flag("repo"), action) {
			out.AllowedActions = append(out.AllowedActions, action)
		}
	}
	for _, rule := range file.Rules {
		if rule.Repo == cx.flag("repo") && rule.ShareID != "" && rule.SharedBy != "" {
			out.Shares = append(out.Shares, shareRule(rule))
		}
	}
	return out, nil
}

func createRepositoryShare(cx *invocation, request kcclient.RepositoryShareRequest) (kcclient.RepositoryShare, error) {
	var result kcclient.RepositoryShare
	if _, err := identity.CanonicalUsername(request.Principal); err != nil {
		return result, err
	}
	policy, err := sharingPolicy(cx)
	if err != nil {
		return result, err
	}
	return createRepositoryShareWithPolicy(cx, request, policy)
}

func createRepositoryShareWithPolicy(cx *invocation, request kcclient.RepositoryShareRequest, policy []string) (kcclient.RepositoryShare, error) {
	var result kcclient.RepositoryShare
	if request.Principal == cx.flag("as") {
		return result, kernel.Fail(kernel.ErrUsageInvalid, "repository sharing requires a different recipient")
	}
	file, err := ReadAllow(cx.Home)
	if err != nil {
		return result, err
	}
	if len(request.Actions) == 0 {
		return result, kernel.Fail(kernel.ErrUsageInvalid, "share requires explicit consumption actions")
	}
	seen := map[string]bool{}
	for _, action := range request.Actions {
		if !slices.Contains(policy, action) || !wholeRepositoryActionAllowed(file.Rules, cx.flag("as"), cx.flag("repo"), action) {
			return result, kernel.Fail(kernel.ErrForbidden, "cannot share action %s beyond deployment policy and current whole-repository permission", action)
		}
		if seen[action] {
			return result, kernel.Fail(kernel.ErrUsageInvalid, "share actions must be unique")
		}
		seen[action] = true
	}
	var random [16]byte
	if _, err := rand.Read(random[:]); err != nil {
		return result, err
	}
	id := "share-" + hex.EncodeToString(random[:])
	rule := AllowRule{ID: id, ShareID: id, SharedBy: cx.flag("as"), Principal: request.Principal, Repo: cx.flag("repo"), Actions: slices.Clone(request.Actions)}
	file.Rules = append(file.Rules, rule)
	if err := WriteAllow(cx.Home, file); err != nil {
		return result, err
	}
	return shareRule(rule), nil
}

func verbRepositoryShareRemove(cx *invocation) (any, error) {
	if _, err := sharingPolicy(cx); err != nil {
		return nil, err
	}
	return removeRepositoryShare(cx)
}

func removeRepositoryShare(cx *invocation) (any, error) {
	file, err := ReadAllow(cx.Home)
	if err != nil {
		return nil, err
	}
	kept := make([]AllowRule, 0, len(file.Rules))
	found := false
	for _, rule := range file.Rules {
		if rule.Repo == cx.flag("repo") && rule.ShareID != "" && rule.ShareID == cx.flag("id") && rule.SharedBy != "" {
			found = true
			continue
		}
		kept = append(kept, rule)
	}
	if !found {
		return nil, kernel.Fail(kernel.ErrUsageInvalid, "no share with this ID in this repository")
	}
	file.Rules = kept
	if err := WriteAllow(cx.Home, file); err != nil {
		return nil, err
	}
	return map[string]any{"repository": cx.flag("repo"), "revoked": cx.flag("id")}, nil
}

func runRemoteAdmissionSharing(ctx context.Context, client *kcclient.Client, path string, flags map[string]FlagValue, options kcclient.RequestOptions) (any, error) {
	var out any
	if path == "admission show" {
		err := client.IdentityService().Admission(ctx, options, &out)
		return out, err
	}
	return nil, kernel.Fail(kernel.ErrUsageInvalid, "unknown admission operation")
}
