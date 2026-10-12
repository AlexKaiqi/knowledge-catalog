package invocation

// Catalog state operations: the read/manage operation bodies that both the
// argv registry and the typed management routes bind by name. (CAP-ORG-01 B6.)

import (
	"fmt"

	"kc/catalog"

	kchome "kc/home"
	"kc/invoke"
	"kc/kernel"
	knowledge "kc/knowledge"
	"kc/snapshot"
	"kc/snapshot/dataset"
)

const (
	DefaultAuditLimit   = 50
	MaxAuditPageSize    = 200
	DefaultHistoryLimit = 50
	MaxHistoryPageSize  = 200
)

type CatalogInventoryItem struct {
	ID string `json:"id"`
}

// catalogListOperation is DiscoverCatalogs: visible Catalog IDs only. Host
// paths stay on the Server; consumers never receive them.
func CatalogListOperation(cx *Invocation) (any, error) {
	var file kchome.HomeFile
	var err error
	if cx.WS != nil {
		file = cx.WS.File
	} else {
		file, err = kchome.ReadHome(cx.Home)
	}
	if err != nil {
		return nil, err
	}
	visible := make([]CatalogInventoryItem, 0, len(file.Catalogs))
	for _, item := range file.Catalogs {
		if OwnerBypass(cx.Flags) || CatalogReadAllowed(cx.Home, invoke.FlagString(cx.Flags, "as"), item.ID, cx.WS) {
			visible = append(visible, CatalogInventoryItem{ID: item.ID})
		}
	}
	return map[string]any{"catalogs": visible}, nil
}

// readCatalogState answers `kc catalog show`: the current combination space,
// not git history (`kc catalog audit`) or local stores (`kc deployment status`).
// The workspaces list member ids only. Registered repositories stay identity
// plus schemaCount; README is not flattened into title/summary.
func ReadCatalogState(cx *Invocation) (any, error) {
	state, err := LoadVisibleCatalogState(cx)
	if err != nil {
		return nil, err
	}
	view := PublicCatalogView(state)
	if id := ConfiguredDiscoveryWorkspace(cx.WS, state.CatalogID); id != "" {
		view["discoveryWorkspaceId"] = id
	}
	view["repositories"] = CatalogRepositoryInventory(cx.WS, state.Repositories)
	return view, nil
}

func LoadVisibleCatalogState(cx *Invocation) (catalog.CatalogState, error) {
	cat, err := PickCatalog(cx.WS, cx.Flags)
	if err != nil {
		return catalog.CatalogState{}, err
	}
	return FilterCatalogState(cx.Home, cx.Flags, cat.DumpState()), nil
}

func PublicCatalogView(state catalog.CatalogState) map[string]any {
	state = catalog.NormalizeCatalogState(state)
	out := map[string]any{
		"catalogId":    state.CatalogID,
		"repositories": state.Repositories,
		"datasets":     PublicKnowledgeSets(state.KnowledgeSets),
	}
	if state.Archived {
		out["archived"] = true
	}
	return out
}

func PublicKnowledgeSets(workspaces []catalog.KnowledgeSet) []map[string]any {
	return dataset.PublicKnowledgeSets(workspaces)
}

func PublicKnowledgeSet(workspace catalog.KnowledgeSet) map[string]any {
	return dataset.PublicKnowledgeSet(workspace)
}

func CatalogIDOf(ws *kchome.Home, flags map[string]invoke.FlagValue) string {
	if id := invoke.FlagString(flags, "catalog"); id != "" {
		return id
	}
	if len(ws.File.Catalogs) > 0 {
		return ws.File.Catalogs[0].ID
	}
	return ""
}

// resolveCurrentCatalog returns the Catalog an embedded operation targets,
// applying the same priority as the remote client: an explicit operand wins,
// then the choice persisted by `kc catalog use`, then the kchome.Home default.
func ResolveCurrentCatalog(cx *Invocation) string {
	catalogID := invoke.FlagString(cx.Flags, "catalog")
	if catalogID == "" && cx.Home != "" {
		if saved := SavedHomeCatalog(cx.Home); saved != "" {
			cx.Flags["catalog"] = saved
			catalogID = saved
		}
	}
	return catalogID
}

func VerbArchiveCatalog(cx *Invocation) (any, error) {
	cat, err := PickCatalog(cx.WS, cx.Flags)
	if err != nil {
		return nil, err
	}
	if err := cat.Archive(); err != nil {
		return nil, err
	}
	return map[string]any{"catalog": CatalogIDOf(cx.WS, cx.Flags), "archived": true}, nil
}

// CatalogRepositoryInventory is the consumer Catalog inventory: registered
// Repository ids plus schemaCount at published HEAD. CatalogState still stores
// ids; this assembly stays in the application layer. README is a knowledge
// object, not title/summary columns on this list.
func CatalogRepositoryInventory(ws *kchome.Home, ids []string) []map[string]any {
	out := make([]map[string]any, 0, len(ids))
	for _, id := range ids {
		out = append(out, DescribeCatalogRepository(ws, kernel.RepositoryID(id)))
	}
	return out
}

func DescribeCatalogRepository(ws *kchome.Home, id kernel.RepositoryID) map[string]any {
	out := map[string]any{"id": string(id)}
	if ws == nil || ws.Reader == nil {
		return out
	}
	repo, err := ws.Reader.Require(id, kernel.ErrCapabilityUnsatisfied)
	if err != nil {
		return out
	}
	commit, err := repo.Head(snapshot.DefaultRef)
	if err != nil {
		return out
	}
	if store, ok := repo.(knowledge.SchemaStore); ok {
		if schemas, schemaErr := store.SchemaObjectIDs(commit); schemaErr == nil {
			out["schemaCount"] = len(schemas)
		}
	}
	return out
}

// FilterCatalogState is the Catalog inventory view. Registration is visible to
// a principal who already passed catalog.read; knowledge.read still gates
// Canonical body, not whether the repository id appears here.
func FilterCatalogState(_ string, _ map[string]invoke.FlagValue, state catalog.CatalogState) catalog.CatalogState {
	return catalog.NormalizeCatalogState(state)
}

func VerbAudit(cx *Invocation) (any, error) {
	limit, err := PageLimit(cx.Flags, DefaultAuditLimit, MaxAuditPageSize)
	if err != nil {
		return nil, err
	}
	layer := cx.Flag("layer")
	cmdFilter := cx.Flag("cmd")
	if layer != "" {
		if layer != "kc" && layer != "system" {
			return nil, fmt.Errorf("--layer must be kc or system")
		}
		entries, err := ReadTrail(cx.Home, layer, cmdFilter, limit)
		if err != nil {
			return nil, err
		}
		return map[string]any{"source": "local", "layer": layer, "entries": entries}, nil
	}
	ws := cx.WS
	if ws == nil {
		var err error
		ws, err = kchome.Open(cx.Home)
		if err != nil {
			entries, trailErr := ReadTrail(cx.Home, "", cmdFilter, limit)
			if trailErr != nil {
				return nil, err
			}
			return map[string]any{"source": "local", "entries": entries}, nil
		}
		defer ws.Close()
	}
	cat, _, err := ws.UseCatalog(ResolveCurrentCatalog(cx))
	if err != nil {
		return nil, err
	}
	hist := cat.Log(catalog.CatalogLogQuery{Limit: limit, Dataset: invoke.DatasetOf(cx.Flags)})
	return map[string]any{
		"source":    "catalog",
		"catalogId": hist.RepositoryID,
		"entries":   CatalogLogEntries(hist.Commits, cmdFilter, limit),
	}, nil
}

const (
	DefaultSchemaPageSize = 50
	MaxSchemaPageSize     = 200
)
