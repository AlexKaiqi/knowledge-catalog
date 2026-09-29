package cli

import (
	kcclient "kc/client"
	inv "kc/invocation"
	"slices"
	"sort"
)

func admissionOverview(cx *inv.Invocation) (kcclient.AdmissionResult, error) {
	out := kcclient.AdmissionResult{
		Principal: cx.Flag("as"),
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
