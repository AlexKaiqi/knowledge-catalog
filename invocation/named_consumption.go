package invocation

import (
	kchome "kc/home"
	"kc/invoke"
	"kc/kernel"
	"kc/snapshot/dataset"
)

const namedConsumptionScopeFlag = "_named-consumption-scope"

// This carrier fixes coordinates for one Invocation, never an authorization
// decision. Every consumer still evaluates the current rules for the full pin.
type namedConsumptionScope struct {
	invocation *Invocation
	datasets   dataset.Service
	request    kernel.Digest
	pin        *dataset.ResolvedDataset
}

func namedConsumptionRequest(flags map[string]invoke.FlagValue) kernel.Digest {
	return kernel.CanonicalDigest([]string{invoke.DatasetOf(flags), invoke.FlagString(flags, RevisionFlag), invoke.FlagString(flags, "pin")})
}

func namedConsumptionRequestAllowed(flags map[string]invoke.FlagValue) bool {
	return invoke.DatasetOf(flags) != "" && invoke.FlagString(flags, "repo") == "" && SuppliedDataset(flags) == nil && !OwnerBypass(flags)
}

func bindNamedConsumptionScope(cx *Invocation) {
	if !namedConsumptionRequestAllowed(cx.Flags) {
		delete(cx.Flags, namedConsumptionScopeFlag)
		return
	}
	if scope, ok := cx.Flags[namedConsumptionScopeFlag].(*namedConsumptionScope); ok && scope.invocation == cx && scope.request == namedConsumptionRequest(cx.Flags) {
		return
	}
	cx.Flags[namedConsumptionScopeFlag] = &namedConsumptionScope{invocation: cx, datasets: cx.WS.Datasets, request: namedConsumptionRequest(cx.Flags)}
}

func namedConsumptionScopeOf(flags map[string]invoke.FlagValue, ws *kchome.Home) *namedConsumptionScope {
	if !namedConsumptionRequestAllowed(flags) {
		return nil
	}
	if scope, ok := flags[namedConsumptionScopeFlag].(*namedConsumptionScope); ok && scope.request == namedConsumptionRequest(flags) {
		return scope
	}
	if ws == nil || ws.Datasets == nil {
		return nil
	}
	scope := &namedConsumptionScope{datasets: ws.Datasets, request: namedConsumptionRequest(flags)}
	flags[namedConsumptionScopeFlag] = scope
	return scope
}

func (s *namedConsumptionScope) resolve(flags map[string]invoke.FlagValue) (dataset.ResolvedDataset, error) {
	if s.pin != nil {
		flags[ResolvedPinFlag] = s.pin.PinID
		return *s.pin, nil
	}
	if s.datasets == nil {
		return dataset.ResolvedDataset{}, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "Dataset service is unavailable")
	}
	id := invoke.DatasetOf(flags)
	revision, err := DatasetRevisionFlag(flags)
	if err != nil {
		return dataset.ResolvedDataset{}, err
	}
	var pin dataset.ResolvedDataset
	if raw := invoke.FlagString(flags, "pin"); raw != "" {
		if revision != 0 {
			return pin, kernel.Fail(kernel.ErrUsageInvalid, "use either --revision or --pin, not both")
		}
		pin, err = ParseReplayPin(raw)
		if err != nil {
			return pin, err
		}
		if pin.SetID != "" && pin.SetID != id {
			return pin, kernel.Fail(kernel.ErrUsageInvalid, "pin names a different dataset")
		}
	} else {
		var definition dataset.Dataset
		if revision == 0 {
			definition, err = s.datasets.Set(id)
		} else {
			definition, err = s.datasets.DatasetVersion(id, revision)
		}
		if err != nil {
			return pin, err
		}
		pin.Revision = definition.Revision
		pin.Repositories = make(map[kernel.RepositoryID]kernel.CommitID, len(definition.Sources))
		for _, source := range definition.Sources {
			pin.Repositories[source.Repository] = source.Commit
		}
	}
	pin.SetID = id
	accepted, err := s.datasets.ReplayPublished(pin)
	if err != nil {
		return dataset.ResolvedDataset{}, err
	}
	s.pin = &accepted
	flags[ResolvedPinFlag] = accepted.PinID
	return accepted, nil
}

// A named Dataset may also be consumed through the same complete fixed-member
// file grants accepted by inventory and the file gateway. Resolve remains its
// own permission; neither object coordinates nor caller pin fields grant access.
func namedDatasetMemberConsumption(home string, flags map[string]invoke.FlagValue, ws *kchome.Home) (dataset.ResolvedDataset, bool, error) {
	var empty dataset.ResolvedDataset
	if !namedConsumptionRequestAllowed(flags) {
		return empty, false, nil
	}
	policy, err := ReadAllow(home)
	if err != nil {
		return empty, false, err
	}
	principal, id := invoke.FlagString(flags, "as"), invoke.DatasetOf(flags)
	rules := withDatasetDefaults(home, principal, policy.Rules)
	if _, ok := MatchAllow(rules, AllowQuery{Principal: principal, Action: "dataset.resolve", Dataset: id}); !ok {
		return empty, false, nil
	}
	scope := namedConsumptionScopeOf(flags, ws)
	if scope == nil {
		return empty, false, nil
	}
	pin, err := scope.resolve(flags)
	if err != nil {
		return empty, false, err
	}
	if len(pin.Repositories) == 0 {
		return empty, false, nil
	}
	for repository, commit := range pin.Repositories {
		if commit == "" {
			return empty, false, nil
		}
		query := AllowQuery{Principal: principal, Action: "file.read", Dataset: id, Repo: string(repository), Ref: string(commit)}
		if _, ok := MatchAllow(rules, query); !ok {
			return empty, false, nil
		}
	}
	return pin, true, nil
}
