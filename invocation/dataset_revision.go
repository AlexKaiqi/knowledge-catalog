package invocation

import (
	"context"
	"encoding/json"
	"sort"
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

// ResolveReadyRevision picks the revision a corpus consumer should use when the
// request names no revision: the newest accepted revision whose corpus is
// already prepared. Acceptance no longer implies readiness, so a Dataset may be
// at v7 while only v6 is searchable; default consumption keeps serving v6 and
// reports the revision it actually used. ok is false when nothing is ready, in
// which case the caller keeps its existing not-prepared answer instead of
// guessing a revision (doc/design/dataset.md#发布).
func ResolveReadyRevision(ctx context.Context, ws *kchome.Home, cat dataset.Service, setID string) (int, bool, error) {
	if ws == nil || ws.Plugins == nil || cat == nil || setID == "" {
		return 0, false, nil
	}
	if !ws.Plugins.Configured() {
		return 0, false, nil
	}
	current, err := cat.Set(setID)
	if err != nil {
		return 0, false, err
	}
	var revisions []int
	for _, version := range cat.DumpState().DatasetVersions {
		if version.SetID != setID || version.Retired || version.Revision > current.Revision {
			continue
		}
		revisions = append(revisions, version.Revision)
	}
	sort.Sort(sort.Reverse(sort.IntSlice(revisions)))
	lookup := ws.Reader.Lookup(cat.Require)
	return newestReadyRevision(ctx, revisions, func(revision int) (bool, error) {
		def, err := cat.DatasetVersion(setID, revision)
		if err != nil {
			return false, nil
		}
		resolved, err := cat.ResolveDefinition(def)
		if err != nil {
			return false, nil
		}
		readiness, err := ws.Plugins.DatasetReadiness(ctx, lookup, kchome.DatasetPin(resolved))
		if err != nil {
			return false, err
		}
		return readiness.Ready, nil
	})
}

// newestReadyRevision walks revisions newest-first and returns the first one
// whose readiness probe answers true. A revision that cannot even be resolved is
// skipped rather than failed: it is simply not a serving candidate.
func newestReadyRevision(ctx context.Context, revisions []int, ready func(int) (bool, error)) (int, bool, error) {
	for _, revision := range revisions {
		if err := ctx.Err(); err != nil {
			return 0, false, err
		}
		ok, err := ready(revision)
		if err != nil {
			return 0, false, err
		}
		if ok {
			return revision, true, nil
		}
	}
	return 0, false, nil
}

// FixReadyRevision is the corpus-consumer half of ResolveReadyRevision. A
// corpus consumer that named no revision is fixed to the newest ready one
// before resolution, so search, relations and traverse agree on a revision that
// can actually be searched and report it back in Dataset.revision. File reads
// never call this: acceptance already makes files effective. An explicit
// --revision or --pin is never overridden, and when nothing is ready the flag
// stays unset so the ordinary not-prepared answer still wins.
func FixReadyRevision(cx *Invocation) error {
	if cx == nil || cx.WS == nil {
		return nil
	}
	flags := cx.Flags
	setID, err := readyRevisionTarget(flags)
	if err != nil || setID == "" {
		return err
	}
	ready, ok, err := ResolveReadyRevision(cx.Context, cx.WS, cx.WS.Datasets, setID)
	if err != nil || !ok {
		return err
	}
	flags[RevisionFlag] = strconv.Itoa(ready)
	return nil
}

// readyRevisionTarget reports the Dataset whose default revision should resolve
// to the newest ready one. An explicit --revision, a caller pin or a temporary
// definition already fixes the scope, so each of them is never a target: the
// user's own coordinate always wins over convenience.
func readyRevisionTarget(flags map[string]invoke.FlagValue) (string, error) {
	if invoke.FlagString(flags, "pin") != "" || SuppliedDataset(flags) != nil {
		return "", nil
	}
	revision, err := DatasetRevisionFlag(flags)
	if err != nil {
		return "", err
	}
	if revision != 0 {
		return "", nil
	}
	return invoke.DatasetOf(flags), nil
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
