package service

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"

	"kc/identity"
	"kc/kernel"
	"kc/snapshot/access"
)

type nativeInventoryPage struct {
	Results    []json.RawMessage `json:"results"`
	Pagination struct {
		HasMore    bool   `json:"has_more"`
		NextOffset string `json:"next_offset"`
	} `json:"pagination"`
}

func nativeInventory(w http.ResponseWriter, r *http.Request, caller identity.HTTPIdentity, resources Resources, target *url.URL, proof http.Header) (any, error) {
	amount := 100
	if raw := r.URL.Query().Get("amount"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 1 || parsed > 1000 {
			return nil, kernel.Fail(kernel.ErrUsageInvalid, "amount must be between 1 and 1000")
		}
		amount = parsed
	}
	visible := []json.RawMessage{}
	ids := []string{}
	query := r.URL.Query()
	query.Set("amount", "1000")
	seen := map[string]bool{}
	for pages := 0; pages < 100; pages++ {
		var page nativeInventoryPage
		if err := fetchNativeJSON(r.Context(), target, "/repositories", query, proof, &page); err != nil {
			return nil, err
		}
		for _, raw := range page.Results {
			var entry struct {
				ID string `json:"id"`
			}
			if json.Unmarshal(raw, &entry) != nil || entry.ID == "" {
				return nil, kernel.Fail(kernel.ErrTemporaryUnavailable, "native repository entry has no identity")
			}
			err := authorizeNative(r.Context(), caller, resources, entry.ID, access.RepositoryMetadataRead, "")
			if kernel.CodeOf(err) == kernel.ErrForbidden {
				continue
			}
			if err != nil {
				return nil, err
			}
			visible = append(visible, raw)
			ids = append(ids, entry.ID)
			if len(visible) > amount {
				return writeNativeInventory(w, visible[:amount], true, ids[amount-1]), nil
			}
		}
		if !page.Pagination.HasMore {
			return writeNativeInventory(w, visible, false, ""), nil
		}
		next := page.Pagination.NextOffset
		if next == "" || next == query.Get("after") || seen[next] {
			return nil, kernel.Fail(kernel.ErrTemporaryUnavailable, "native inventory pagination did not advance")
		}
		seen[next] = true
		query.Set("after", next)
	}
	return nil, kernel.Fail(kernel.ErrTemporaryUnavailable, "native inventory scan budget exceeded")
}

func writeNativeInventory(w http.ResponseWriter, entries []json.RawMessage, more bool, next string) any {
	result := map[string]any{"results": entries, "pagination": map[string]any{"has_more": more, "next_offset": next, "results": len(entries), "max_per_page": 1000}}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, result)
	return map[string]any{"repositories": len(entries)}
}
