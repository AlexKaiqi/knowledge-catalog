package service

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strings"

	"kc/identity"
	"kc/kernel"
	"kc/snapshot/access"
)

// Storage privileges are wider than caller privileges. Import and physical
// linking must check every intersecting repository namespace before forwarding.
// Unregistered locations have no product owner and cannot be authorized here.
func nativePhysicalSourceChecks(r *http.Request, caller identity.HTTPIdentity, resources Resources, target *url.URL, proof http.Header, route nativeRoute, body map[string]json.RawMessage) error {
	var locations []struct {
		Path string `json:"path"`
		Type string `json:"type"`
	}
	if strings.HasSuffix(route.path, "/import") {
		if json.Unmarshal(body["paths"], &locations) != nil || len(locations) == 0 {
			return kernel.Fail(kernel.ErrUsageInvalid, "import requires source locations")
		}
	} else {
		var staging struct {
			PhysicalAddress string `json:"physical_address"`
		}
		if json.Unmarshal(body["staging"], &staging) != nil || staging.PhysicalAddress == "" {
			return kernel.Fail(kernel.ErrUsageInvalid, "link requires a staging location")
		}
		locations = append(locations, struct {
			Path string `json:"path"`
			Type string `json:"type"`
		}{staging.PhysicalAddress, "object"})
	}
	for _, location := range locations {
		if location.Type != "object" && location.Type != "common_prefix" {
			return kernel.Fail(kernel.ErrUsageInvalid, "invalid import source type")
		}
		if err := authorizePhysicalSource(r, caller, resources, target, proof, location.Path, location.Type == "common_prefix"); err != nil {
			return err
		}
	}
	return nil
}

func physicalLocation(raw string) (*url.URL, error) {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme == "" || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || !safeNativePath(u.Path) {
		return nil, kernel.Fail(kernel.ErrForbidden, "physical source has no safe repository coordinates")
	}
	return u, nil
}

func physicalContains(parent, child *url.URL) bool {
	return parent.Scheme == child.Scheme && parent.Host == child.Host &&
		(child.Path == strings.TrimRight(parent.Path, "/") || strings.HasPrefix(child.Path, strings.TrimRight(parent.Path, "/")+"/"))
}

func authorizePhysicalSource(r *http.Request, caller identity.HTTPIdentity, resources Resources, target *url.URL, proof http.Header, raw string, prefix bool) error {
	source, err := physicalLocation(raw)
	if err != nil {
		return err
	}
	query := url.Values{"amount": {"1000"}}
	seen := map[string]bool{}
	matched := false
	for pages := 0; pages < 100; pages++ {
		var page nativeInventoryPage
		if err := fetchNativeJSON(r.Context(), target, "/repositories", query, proof, &page); err != nil {
			return err
		}
		for _, entry := range page.Results {
			var repository struct {
				ID        string `json:"id"`
				Namespace string `json:"storage_namespace"`
			}
			if json.Unmarshal(entry, &repository) != nil || repository.ID == "" {
				return kernel.Fail(kernel.ErrTemporaryUnavailable, "invalid native repository coordinates")
			}
			namespace, err := physicalLocation(repository.Namespace)
			if err != nil {
				return kernel.Fail(kernel.ErrTemporaryUnavailable, "invalid native repository namespace")
			}
			if !physicalContains(namespace, source) && !(prefix && physicalContains(source, namespace)) {
				continue
			}
			matched = true
			// Physical coordinates do not establish a source ref. A ref-limited
			// grant therefore cannot authorize an arbitrary backing-store read.
			if err := authorizeNative(r.Context(), caller, resources, repository.ID, access.RepositoryFileRead, ""); err != nil {
				return err
			}
		}
		if !page.Pagination.HasMore {
			if !matched {
				return kernel.Fail(kernel.ErrForbidden, "physical source is outside registered repository authorization")
			}
			return nil
		}
		next := page.Pagination.NextOffset
		if next == "" || seen[next] {
			return kernel.Fail(kernel.ErrTemporaryUnavailable, "native source inventory did not advance")
		}
		seen[next] = true
		query.Set("after", next)
	}
	return kernel.Fail(kernel.ErrTemporaryUnavailable, "native source inventory scan budget exceeded")
}
