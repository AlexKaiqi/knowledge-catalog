package invocation

import (
	"encoding/json"
	"strconv"
	"strings"

	kchome "kc/home"
	"kc/kernel"
	"kc/snapshot/dataset"

	"kc/invoke"
)

// RevisionFlag fixes a named Dataset read to one accepted revision. Every
// consuming step of one task passes the revision its first step returned, so
// list, describe, search, read and relations see the same frozen sources.
const RevisionFlag = "revision"

// DatasetRevisionFlag parses --revision. Zero means "the current revision".
func DatasetRevisionFlag(flags map[string]invoke.FlagValue) (int, error) {
	raw := strings.TrimSpace(invoke.FlagString(flags, RevisionFlag))
	if raw == "" {
		return 0, nil
	}
	revision, err := strconv.Atoi(raw)
	if err != nil || revision <= 0 {
		return 0, kernel.Fail(kernel.ErrUsageInvalid, "--revision must be a positive integer")
	}
	return revision, nil
}

// ResolveAtRevision resolves the request's Dataset scope. With --revision it
// turns the accepted revision's frozen sources into a pin and replays it, so
// the normal published-pin checks (retirement, membership, availability and
// per-source authorization) still decide; a revision never widens scope.
func ResolveAtRevision(ws *kchome.Home, home string, cat dataset.Service, setID string, flags map[string]invoke.FlagValue) (dataset.ResolvedDataset, error) {
	if setID == invoke.DatasetOf(flags) {
		if scope := namedConsumptionScopeOf(flags, nil); scope != nil && scope.pin != nil {
			return scope.resolve(flags)
		}
	}
	revision, err := DatasetRevisionFlag(flags)
	if err != nil {
		return dataset.ResolvedDataset{}, err
	}
	if revision == 0 {
		return ResolveOrReplay(ws, home, cat, setID, flags)
	}
	if setID == "" || SuppliedDataset(flags) != nil {
		return dataset.ResolvedDataset{}, kernel.Fail(kernel.ErrUsageInvalid, "--revision fixes a named Dataset; pass --dataset")
	}
	if invoke.FlagString(flags, "pin") != "" {
		return dataset.ResolvedDataset{}, kernel.Fail(kernel.ErrUsageInvalid, "use either --revision or --pin, not both")
	}
	if cat == nil {
		return dataset.ResolvedDataset{}, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "dataset service is unavailable")
	}
	definition, err := cat.DatasetVersion(setID, revision)
	if err != nil {
		return dataset.ResolvedDataset{}, err
	}
	pin, err := cat.ResolveDefinition(definition)
	if err != nil {
		return dataset.ResolvedDataset{}, err
	}
	raw, err := json.Marshal(dataset.ResolvedDataset{SetID: pin.SetID, Revision: pin.Revision, Repositories: pin.Repositories, PinID: pin.PinID})
	if err != nil {
		return dataset.ResolvedDataset{}, err
	}
	scoped := make(map[string]invoke.FlagValue, len(flags)+1)
	for name, value := range flags {
		scoped[name] = value
	}
	scoped["pin"] = string(raw)
	resolved, err := ResolveOrReplay(ws, home, cat, setID, scoped)
	if value, ok := scoped[ResolvedPinFlag]; ok && err == nil {
		flags[ResolvedPinFlag] = value
	}
	return resolved, err
}
