package service

import (
	"context"
	"net/http"
	"slices"
	"sort"
	"strings"

	"kc/identity"
	"kc/kernel"
	"kc/snapshot/access"
)

type grantRequest struct {
	Principal  string   `json:"principal"`
	Actions    []string `json:"actions"`
	Repository string   `json:"repository,omitempty"`
	Catalog    string   `json:"catalog,omitempty"`
	Ref        string   `json:"ref,omitempty"`
	Object     string   `json:"object,omitempty"`
	Aspect     string   `json:"aspect,omitempty"`
	Dataset    string   `json:"dataset,omitempty"`
}

func requiredAccess(resources Resources) (*access.Service, error) {
	if resources.Access == nil || resources.Access.Store == nil {
		return nil, kernel.Fail(kernel.ErrPreconditionFailed, "durable authorization policy is unavailable")
	}
	return resources.Access, nil
}

// Preserve the established HTTP-to-CLI action normalization without using a
// CLI executor or interpreting application action names.
func grantActions(actions []string) []string {
	out := []string{}
	for _, action := range strings.Split(strings.Join(actions, ","), ",") {
		if action = strings.TrimSpace(action); action != "" {
			out = append(out, action)
		}
	}
	return out
}

func (h *Handler) adminGrantList(w http.ResponseWriter, r *http.Request) {
	filter, err := grantFilter(r)
	if err != nil {
		writeError(w, err)
		return
	}
	h.withResources(w, r, "admin-grant-list", func(ctx context.Context, caller identity.HTTPIdentity, resources Resources) (any, error) {
		service, err := requiredAccess(resources)
		if err != nil {
			return nil, err
		}
		return service.List(ctx, caller.Principal, filter)
	})
}

func (h *Handler) adminGrantAdd(w http.ResponseWriter, r *http.Request) {
	var request grantRequest
	if !decodeRequest(w, r, &request) {
		return
	}
	h.withResources(w, r, "admin-grant-add", func(ctx context.Context, caller identity.HTTPIdentity, resources Resources) (any, error) {
		service, err := requiredAccess(resources)
		if err != nil {
			return nil, err
		}
		return service.Add(ctx, caller.Principal, access.Rule{
			Principal: request.Principal, Actions: grantActions(request.Actions), Repo: request.Repository, Catalog: request.Catalog,
			Ref: request.Ref, Object: request.Object, Aspect: request.Aspect, Dataset: request.Dataset,
		})
	})
}

func (h *Handler) adminGrantRemove(w http.ResponseWriter, r *http.Request) {
	if !decodeEmptyRequest(w, r) {
		return
	}
	h.withResources(w, r, "admin-grant-remove", func(ctx context.Context, caller identity.HTTPIdentity, resources Resources) (any, error) {
		service, err := requiredAccess(resources)
		if err != nil {
			return nil, err
		}
		id := r.PathValue("grant")
		if err := service.Revoke(ctx, caller.Principal, id); err != nil {
			return nil, err
		}
		return map[string]any{"revoked": id}, nil
	})
}

type admissionGrant struct {
	ID         string   `json:"id"`
	Actions    []string `json:"actions"`
	Catalog    string   `json:"catalog,omitempty"`
	Repository string   `json:"repository,omitempty"`
	Dataset    string   `json:"dataset,omitempty"`
	Ref        string   `json:"ref,omitempty"`
	Object     string   `json:"object,omitempty"`
	Aspect     string   `json:"aspect,omitempty"`
	SharedBy   string   `json:"sharedBy,omitempty"`
}

type admissionResult struct {
	Principal string           `json:"principal"`
	Grants    []admissionGrant `json:"grants"`
	Request   struct {
		URL            string   `json:"url,omitempty"`
		Administrators []string `json:"administrators"`
	} `json:"request"`
}

func (h *Handler) admissionShow(w http.ResponseWriter, r *http.Request) {
	h.withResources(w, r, "admission-show", func(ctx context.Context, caller identity.HTTPIdentity, resources Resources) (any, error) {
		service, err := requiredAccess(resources)
		if err != nil {
			return nil, err
		}
		rules, err := service.Store.ReadRules(ctx)
		if err != nil {
			return nil, err
		}
		return admissionOverview(caller.Principal, rules, resources.RequestURL), nil
	})
}

func admissionOverview(principal string, rules []access.Rule, requestURL string) admissionResult {
	out := admissionResult{Principal: principal, Grants: []admissionGrant{}}
	out.Request.URL = requestURL
	out.Request.Administrators = []string{}
	for _, rule := range access.Inventory(rules, access.Query{Principal: principal}).Rules {
		out.Grants = append(out.Grants, admissionGrant{
			ID: rule.ID, Actions: slices.Clone(rule.Actions), Catalog: rule.Catalog,
			Repository: rule.Repo, Dataset: rule.Dataset, Ref: rule.Ref,
			Object: rule.Object, Aspect: rule.Aspect, SharedBy: rule.SharedBy,
		})
	}
	administrators := map[string]bool{}
	for _, rule := range rules {
		for _, action := range rule.Actions {
			if access.ActionMatches(action, "admin.grants.manage") {
				administrators[rule.Principal] = true
				break
			}
		}
	}
	for name := range administrators {
		out.Request.Administrators = append(out.Request.Administrators, name)
	}
	sort.Strings(out.Request.Administrators)
	return out
}
