package service

import (
	"net/http"
	"net/url"

	"kc/kernel"
	"kc/snapshot/access"
)

func grantFilter(r *http.Request) (access.Query, error) {
	query, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil {
		return access.Query{}, kernel.Fail(kernel.ErrUsageInvalid, "invalid grant filter: %v", err)
	}
	filter := access.Query{}
	names := map[string]*string{"principal": &filter.Principal, "action": &filter.Action,
		"repository": &filter.Repo, "catalog": &filter.Catalog, "dataset": &filter.Dataset,
		"ref": &filter.Ref, "object": &filter.Object, "aspect": &filter.Aspect}
	for name, values := range query {
		field, ok := names[name]
		if !ok || len(values) != 1 {
			return access.Query{}, kernel.Fail(kernel.ErrUsageInvalid, "unknown or repeated grant filter %s", name)
		}
		*field = values[0]
	}
	return filter, nil
}
