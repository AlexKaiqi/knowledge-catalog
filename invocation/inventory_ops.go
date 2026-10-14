package invocation

// Registry state operations: the read/manage operation bodies that both the
// argv registry and the typed management routes bind by name. (CAP-ORG-01 B6.)

import (
	"fmt"

	"kc/snapshot/dataset"

	kchome "kc/home"
	"kc/invoke"
	"kc/kernel"
	objectmodel "kc/object"
	"kc/snapshot"
)

const (
	DefaultAuditLimit   = 50
	MaxAuditPageSize    = 200
	DefaultHistoryLimit = 50
	MaxHistoryPageSize  = 200
)

// readRegistryState answers `kc show`: the current combination space,
// not git history (`kc registry audit`) or local stores (`kc deployment status`).
// The workspaces list member ids only. Registered repositories stay identity
// plus schemaCount; README is not flattened into title/summary.
func ReadInventoryState(cx *Invocation) (any, error) {
	state, err := LoadInventoryState(cx)
	if err != nil {
		return nil, err
	}
	view := InventoryView(state)
	if id := ConfiguredDiscoveryWorkspace(cx.WS); id != "" {
		view["discoveryWorkspaceId"] = id
	}
	view["repositories"] = RepositoryInventory(cx.WS, state.Repositories)
	return view, nil
}

func LoadInventoryState(cx *Invocation) (dataset.State, error) {
	cat, err := PickDatasets(cx.WS, cx.Flags)
	if err != nil {
		return dataset.State{}, err
	}
	return FilterInventoryState(cx.Home, cx.Flags, cat.DumpState()), nil
}

func InventoryView(state dataset.State) map[string]any {
	state = dataset.NormalizeState(state)
	out := map[string]any{
		"repositories": state.Repositories,
		"datasets":     PublicKnowledgeSets(state.KnowledgeSets),
	}
	return out
}

func PublicKnowledgeSets(workspaces []dataset.KnowledgeSet) []map[string]any {
	return dataset.PublicKnowledgeSets(workspaces)
}

func PublicKnowledgeSet(workspace dataset.KnowledgeSet) map[string]any {
	return dataset.PublicKnowledgeSet(workspace)
}

// RepositoryInventory is the consumer Registry inventory: registered
// Repository ids plus schemaCount at published HEAD. RegistryState still stores
// ids; this assembly stays in the application layer. README is a knowledge
// object, not title/summary columns on this list.
func RepositoryInventory(ws *kchome.Home, ids []string) []map[string]any {
	out := make([]map[string]any, 0, len(ids))
	for _, id := range ids {
		out = append(out, DescribeRepository(ws, kernel.RepositoryID(id)))
	}
	return out
}

func DescribeRepository(ws *kchome.Home, id kernel.RepositoryID) map[string]any {
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
	if store, ok := repo.(objectmodel.SchemaStore); ok {
		if schemas, schemaErr := store.SchemaObjectIDs(commit); schemaErr == nil {
			out["schemaCount"] = len(schemas)
		}
	}
	return out
}

// FilterInventoryState is the Registry inventory view. Registration is visible to
// a principal who already passed inventory.read; objectmodel.read still gates
// Canonical body, not whether the repository id appears here.
func FilterInventoryState(_ string, _ map[string]invoke.FlagValue, state dataset.State) dataset.State {
	return dataset.NormalizeState(state)
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
	cat, err := PickDatasets(ws, cx.Flags)
	if err != nil {
		return nil, err
	}
	hist, err := cat.Log(dataset.LogQuery{Limit: limit, Dataset: invoke.DatasetOf(cx.Flags)})
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"source":      "registry",
		"authorityId": hist.RepositoryID,
		"entries":     RegistryLogEntries(hist.Commits, cmdFilter, limit),
	}, nil
}

const (
	DefaultSchemaPageSize = 50
	MaxSchemaPageSize     = 200
)
