package dataset

import (
	"encoding/base64"
	"encoding/json"
	"slices"

	"kc/kernel"
)

const (
	defaultManagementHistoryLimit = 50
	maxManagementHistoryPageSize  = 200
)

// ManagementOptions fixes the page size and accepted-history continuation.
// Limit zero uses the existing default of 50; the maximum remains 200.
type ManagementOptions struct {
	Limit        int
	Continuation string
}

// ManagementView reads accepted registry records, independently of file
// consumption. Historical metadata inherits the current lifecycle verdict.
func ManagementView(state CatalogState, id string, revision int, options ManagementOptions) (map[string]any, error) {
	current, selected, versions, err := datasetManagementRecords(state, id, revision)
	if err != nil {
		return nil, err
	}
	page, continuation, err := datasetVersionPage(state.CatalogID, id, current.Revision, versions, options)
	if err != nil {
		return nil, err
	}
	view := PublicKnowledgeSet(selected)
	view["servingRevision"] = current.Revision
	view["versions"] = page
	if continuation != "" {
		view["continuation"] = continuation
	}
	delete(view, "retired")
	if current.Retired {
		view["retired"] = true
	}
	items := selected.Items
	if items == nil {
		items = []DatasetItem{}
	}
	sources := selected.Sources
	if sources == nil {
		sources = []KnowledgeSetSource{}
	}
	view["items"], view["sources"] = items, sources
	return view, nil
}

func datasetManagementRecords(state CatalogState, id string, revision int) (KnowledgeSet, KnowledgeSet, []int, error) {
	var current *KnowledgeSet
	for i := range state.KnowledgeSets {
		if state.KnowledgeSets[i].SetID == id {
			current = &state.KnowledgeSets[i]
			break
		}
	}
	if current == nil {
		return KnowledgeSet{}, KnowledgeSet{}, nil, kernel.Fail(kernel.ErrKnowledgeSetInvalid, "dataset %s is not visible", id)
	}
	selected := *current
	versions := []int{current.Revision}
	found := revision == 0 || revision == current.Revision
	for _, record := range state.DatasetVersions {
		if record.SetID != id {
			continue
		}
		versions = append(versions, record.Revision)
		if record.Revision == revision {
			selected = record
			found = true
		}
	}
	if !found {
		return KnowledgeSet{}, KnowledgeSet{}, nil, kernel.Fail(kernel.ErrVersionUnresolved, "dataset %s v%d was not published", id, revision)
	}
	slices.Sort(versions)
	return *current, selected, slices.Compact(versions), nil
}

type datasetHistoryCursor struct {
	Version int           `json:"version"`
	Catalog string        `json:"catalog"`
	Dataset string        `json:"dataset"`
	Serving int           `json:"serving"`
	Before  int           `json:"before"`
	Check   kernel.Digest `json:"check"`
}

func datasetHistoryCursorCheck(cursor datasetHistoryCursor) kernel.Digest {
	cursor.Check = ""
	return kernel.CanonicalDigest(cursor)
}

func decodeDatasetHistoryCursor(raw, catalogID, datasetID string, serving int) (int, error) {
	if raw == "" {
		return 0, nil
	}
	decoded, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil {
		return 0, kernel.Fail(kernel.ErrUsageInvalid, "dataset history continuation is invalid")
	}
	var cursor datasetHistoryCursor
	if err := json.Unmarshal(decoded, &cursor); err != nil || cursor.Version != 1 ||
		cursor.Catalog != catalogID || cursor.Dataset != datasetID || cursor.Serving != serving ||
		cursor.Before <= 0 || cursor.Before > serving || cursor.Check != datasetHistoryCursorCheck(cursor) {
		return 0, kernel.Fail(kernel.ErrUsageInvalid, "dataset history continuation does not match the fixed Catalog and Dataset basis; reload history")
	}
	return cursor.Before, nil
}

func datasetVersionPage(catalogID, datasetID string, serving int, versions []int, options ManagementOptions) ([]int, string, error) {
	limit := options.Limit
	if limit == 0 {
		limit = defaultManagementHistoryLimit
	}
	if limit < 1 || limit > maxManagementHistoryPageSize {
		return nil, "", kernel.Fail(kernel.ErrUsageInvalid, "dataset history limit must be between 1 and %d", maxManagementHistoryPageSize)
	}
	before, err := decodeDatasetHistoryCursor(options.Continuation, catalogID, datasetID, serving)
	if err != nil {
		return nil, "", err
	}
	end := len(versions)
	if before > 0 {
		var found bool
		end, found = slices.BinarySearch(versions, before)
		if !found {
			return nil, "", kernel.Fail(kernel.ErrUsageInvalid, "dataset history continuation names an unpublished revision")
		}
	}
	start := max(0, end-limit)
	continuation := ""
	if start > 0 {
		cursor := datasetHistoryCursor{Version: 1, Catalog: catalogID, Dataset: datasetID, Serving: serving, Before: versions[start]}
		cursor.Check = datasetHistoryCursorCheck(cursor)
		raw, _ := json.Marshal(cursor)
		continuation = base64.RawURLEncoding.EncodeToString(raw)
	}
	return versions[start:end], continuation, nil
}
