package server

import (
	"net/http"
	"net/url"

	"kc/kernel"
)

func grantListFlags(r *http.Request) (map[string]FlagValue, error) {
	query, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil {
		return nil, kernel.Fail(kernel.ErrUsageInvalid, "invalid grant filter: %v", err)
	}
	names := map[string]string{"principal": "principal", "action": "action", "repository": "repo", "catalog": "catalog", "dataset": "dataset", "ref": "ref", "object": "object", "aspect": "aspect"}
	flags := map[string]FlagValue{}
	for key, values := range query {
		name, ok := names[key]
		if !ok || len(values) != 1 {
			return nil, kernel.Fail(kernel.ErrUsageInvalid, "unknown or repeated grant filter %s", key)
		}
		if values[0] != "" {
			flags[name] = values[0]
		}
	}
	return flags, nil
}
