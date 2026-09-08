package cli

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"slices"
	"strings"

	kcclient "kc/client"
	apphome "kc/home"
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
	mux.HandleFunc("POST /identity/v1/admission", f.admissionRequest)
	mux.HandleFunc("GET /catalog/v1/repositories/{repository}/shares", f.repositoryShareList)
	mux.HandleFunc("POST /catalog/v1/repositories/{repository}/shares", f.repositoryShareCreate)
	mux.HandleFunc("DELETE /catalog/v1/repositories/{repository}/shares/{share}", f.repositoryShareRevoke)
}

func (f *httpFacade) admissionShow(w http.ResponseWriter, r *http.Request) {
	f.executeTyped(w, r, "admission-show", "identity.read", command{stage: stageHome, run: func(cx *invocation) (any, error) {
		return admissionDecision(cx, f.admissionHuman(cx), false)
	}}, map[string]FlagValue{})
}

func (f *httpFacade) admissionRequest(w http.ResponseWriter, r *http.Request) {
	if !decodeEmptyServiceRequest(w, r) {
		return
	}
	f.executeTyped(w, r, "admission-request", "identity.admission.request", command{stage: stageHome, run: func(cx *invocation) (any, error) {
		return admissionDecision(cx, f.admissionHuman(cx), true)
	}}, map[string]FlagValue{})
}

func (f *httpFacade) admissionHuman(cx *invocation) bool {
	if _, err := identity.CanonicalUsername(cx.flag("as")); err != nil || cx.flag("on-behalf-of") != "" {
		return false
	}
	return f.options.localAssertion() || (cx.flag("_identity-provider") != "" && cx.flag("_identity-subject") != "" && cx.flag("_identity-issuer") != "")
}

func admissionDecision(cx *invocation, human, apply bool) (kcclient.AdmissionResult, error) {
	out := kcclient.AdmissionResult{Principal: cx.flag("as"), Status: "DISABLED", Actions: []string{}, CurrentActions: []string{}}
	var policy *apphome.AdmissionConfig
	if cx.WS != nil && cx.WS.Deployment != nil {
		policy = cx.WS.Deployment.Admission
	}
	if policy == nil || !policy.Enabled {
		if apply {
			return out, kernel.Fail(kernel.ErrForbidden, "self-service admission is not enabled")
		}
		return out, nil
	}
	out.Catalog, out.Eligible = policy.Catalog, human && policy.Allows(out.Principal)
	if out.Eligible {
		out.Status, out.Actions = "AVAILABLE", slices.Clone(policy.Actions)
	} else {
		out.Status = "NOT_ELIGIBLE"
	}
	file, err := ReadAllow(cx.Home)
	if err != nil {
		return out, err
	}
	key := string(kernel.CanonicalDigest([]string{out.Principal, policy.Catalog}))
	if receipt, exists := file.Admissions[key]; exists {
		if receipt.Principal != out.Principal || receipt.Catalog != policy.Catalog {
			return out, kernel.Fail(kernel.ErrPreconditionFailed, "admission receipt conflicts with its identity")
		}
		out.Status, out.Actions = "ADMITTED", slices.Clone(receipt.Actions)
		if apply {
			out.Status = "REPLAYED"
		}
	} else if apply {
		if !out.Eligible {
			return out, kernel.Fail(kernel.ErrForbidden, "current human user is not eligible for this admission policy")
		}
		file.Rules = append(file.Rules, AllowRule{ID: "admission-" + key, Principal: out.Principal, Catalog: policy.Catalog, Actions: slices.Clone(policy.Actions)})
		if file.Admissions == nil {
			file.Admissions = map[string]AdmissionReceipt{}
		}
		file.Admissions[key] = AdmissionReceipt{Principal: out.Principal, Catalog: policy.Catalog, Actions: slices.Clone(policy.Actions)}
		if err := WriteAllow(cx.Home, file); err != nil {
			return out, err
		}
		out.Status = "APPLIED"
	}
	for _, action := range out.Actions {
		if _, ok := MatchAllow(file.Rules, AllowQuery{Principal: out.Principal, Catalog: policy.Catalog, Action: action}); ok {
			out.CurrentActions = append(out.CurrentActions, action)
		}
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
		if rule.Principal != principal || (rule.Repo != "" && rule.Repo != repository) || rule.Catalog != "" || rule.Ref != "" || rule.Object != "" || rule.Aspect != "" || rule.Workspace != "" {
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
	if path == "admission request" {
		err := client.IdentityService().RequestAdmission(ctx, options, &out)
		return out, err
	}
	repository, err := requireRemoteFlag(flags, "repo")
	if err != nil {
		return nil, err
	}
	service := client.CatalogService()
	switch path {
	case "catalog repo share list":
		err = service.RepositoryShares(ctx, repository, options, &out)
	case "catalog repo share add":
		var principal string
		principal, err = requireRemoteFlag(flags, "principal")
		if err == nil {
			err = service.ShareRepository(ctx, repository, kcclient.RepositoryShareRequest{Principal: principal, Actions: strings.FieldsFunc(FlagString(flags, "action"), func(r rune) bool { return r == ',' })}, options, &out)
		}
	case "catalog repo share remove":
		var id string
		id, err = requireRemoteFlag(flags, "id")
		if err == nil {
			err = service.RevokeRepositoryShare(ctx, repository, id, options, &out)
		}
	default:
		return nil, kernel.Fail(kernel.ErrUsageInvalid, "unknown admission/share operation")
	}
	return out, err
}
