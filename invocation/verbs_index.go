package invocation

import (
	discoveryservice "kc/discovery/service"
	kchome "kc/home"
	objectserving "kc/liveaccess"

	index "kc/discovery/projection"
	"kc/invoke"
	"kc/kernel"
	objectmodel "kc/object"
	"kc/object/reader"

	"encoding/json"
	"sort"
)

func VerbSearch(cx *Invocation) (any, error) {
	if err := RequireDatasetDiscoveryScope(cx.Flags); err != nil {
		return nil, err
	}
	return SearchWorkspace(cx)
}

// RequireDatasetDiscoveryScope is shared by the typed request and embedded
// task paths: a pin is a version coordinate, never an implicit search scope.
func RequireDatasetDiscoveryScope(flags map[string]invoke.FlagValue) error {
	if flags[InventoryDiscoveryFlag] == true {
		return kernel.Fail(kernel.ErrUsageInvalid, "inventoryDiscovery is retired; select a Dataset and use its consumption permission")
	}
	if invoke.FlagString(flags, "repo") != "" || invoke.FlagString(flags, "commit") != "" || invoke.FlagString(flags, "ref") != "" {
		return kernel.Fail(kernel.ErrUsageInvalid, "search, relations and traverse require --dataset; repository search is retired")
	}
	if invoke.DatasetOf(flags) == "" || SuppliedDataset(flags) != nil {
		return kernel.Fail(kernel.ErrUsageInvalid, "select a named --dataset and its published revision")
	}
	return nil
}

func VerbDescribeIndex(cx *Invocation) (any, error) {
	return datasetIndexOperation(cx, indexOperationDescribe)
}

func VerbIndexSync(cx *Invocation) (any, error) {
	return datasetIndexOperation(cx, indexOperationPrepare)
}

func VerbIndexReady(cx *Invocation) (any, error) {
	return datasetIndexOperation(cx, indexOperationReady)
}

type indexOperation int

const (
	indexOperationReady indexOperation = iota
	indexOperationPrepare
	indexOperationDescribe
)

func datasetIndexOperation(cx *Invocation, operation indexOperation) (any, error) {
	if err := RequireDatasetDiscoveryScope(cx.Flags); err != nil {
		return nil, err
	}
	serving, cat, err := OpenServingAt(cx, cx.WS, cx.Flags)
	if err != nil {
		return nil, err
	}
	pin := serving.Pin()
	if operation != indexOperationPrepare {
		if err := RequireCompleteWorkspaceRead(cx.Home, cx.Flags, pin, ""); err != nil {
			return nil, err
		}
	}
	if cx.WS.Plugins == nil || !cx.WS.Plugins.Configured() {
		return nil, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "Dataset index plugin is not configured")
	}
	lookup := cx.WS.Reader.Lookup(cat.Require)
	switch operation {
	case indexOperationReady:
		return cx.WS.Plugins.DatasetReadiness(cx.Context, lookup, pin)
	case indexOperationDescribe:
		diagnosis, err := cx.WS.Plugins.DatasetDiagnosis(cx.Context, lookup, pin)
		if err != nil {
			return nil, err
		}
		return datasetIndexReport(pin, diagnosis, nil), nil
	}
	return datasetPrepareOperation(cx, lookup, pin)
}

// datasetIndexReport is the single shape kc index prepare and kc index
// describe answer with: the fixed source vector, the readiness answer, and the
// corpus this revision froze. Members keep their source commit, because a pin
// is a version coordinate rather than a repository HEAD. Prepare adds the
// observation generation its refresh published; describe adds why the last
// attempt stopped.
func datasetIndexReport(pin reader.DatasetPin, diagnosis index.IndexDiagnosis, observation *index.StateSync) map[string]any {
	stages := make(map[kernel.RepositoryID]index.IndexMemberState, len(diagnosis.Members))
	for _, member := range diagnosis.Members {
		stages[member.Repository] = member
	}
	ids := make([]kernel.RepositoryID, 0, len(pin.Repositories))
	for id := range pin.Repositories {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	members := make([]map[string]any, 0, len(ids))
	for _, id := range ids {
		row := map[string]any{"repository": id, "commit": pin.Repositories[id]}
		if stage, ok := stages[id]; ok {
			row["stage"], row["objectCount"] = stage.Stage, stage.ObjectCount
		}
		members = append(members, row)
	}
	out := map[string]any{"dataset": diagnosis.Dataset, "revision": diagnosis.Revision,
		"ready": diagnosis.Ready, "members": members}
	if diagnosis.Basis != nil {
		out["basis"] = diagnosis.Basis
	}
	if diagnosis.Failure != "" {
		out["failure"] = diagnosis.Failure
	}
	if observation != nil {
		// The pin already fixes the basis commit, so the refresh reports only
		// the observation it published, not a member coordinate.
		if facts, err := datasetProjectionFacts(observation); err == nil {
			out["observation"] = facts
		}
	}
	return out
}

// datasetPrepareOperation builds the accepted revision's corpus on request. It
// is the explicit retry path: publication no longer waits for a build, and the
// background lane is what normally builds this same corpus. Both run the one
// build function the instance publishes, so the retry path cannot diverge from
// the lane (doc/design/dataset.md#发布).
func datasetPrepareOperation(cx *Invocation, lookup reader.MemberLookup, pin reader.DatasetPin) (any, error) {
	if len(pin.Repositories) == 0 {
		return nil, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "Dataset has no selected index sources")
	}
	// Explicit preparation is the retry path, so it also republishes the
	// observation of the revision it just made searchable. A revision whose
	// corpus only now exists has no observation to report otherwise. The
	// caller's own runtime stands in when this process has no worker, because
	// an explicit refresh may observe its own source without a configured
	// worker (doc/design/discovery.md#观察).
	runtime, err := ResourceLookup(cx)
	if err != nil {
		return nil, err
	}
	var stateRequest discoveryservice.ProjectionStateRequest
	if runtime != nil {
		stateRequest = func() (objectmodel.RequestContext, error) { return stateRequestContextFrom(cx) }
	}
	prepared, err := (discoveryservice.DatasetPrepareExecutor{Plugins: cx.WS.Plugins}).Execute(cx.Context, discoveryservice.DatasetPrepareRequest{
		Pin: pin, Lookup: lookup, State: runtime, StateRequest: stateRequest,
	})
	if err != nil {
		return nil, err
	}
	// The caller gets the accepted revision's answer, not a provider
	// coordinate: what it can search now and what it froze.
	diagnosis, err := cx.WS.Plugins.DatasetDiagnosis(cx.Context, lookup, pin)
	if err != nil {
		return nil, err
	}
	observation := prepared.Observation
	return datasetIndexReport(pin, diagnosis, &observation), nil
}

// The Dataset pin is the public authority basis. Provider corpus coordinates
// are derived identities, so diagnostics report facts without pseudo-Repos.
func datasetProjectionFacts(value any) (map[string]any, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	var facts map[string]any
	if err := json.Unmarshal(raw, &facts); err != nil {
		return nil, err
	}
	for _, key := range []string{"repository", "basisRepository", "basisCommit", "headCommit", "lagBehindHead"} {
		delete(facts, key)
	}
	return facts, nil

}

// VerbDescribeAccess reports one logical AccessSpec per pinned member, or the
// single AccessSpec of one Repository's published basis.
func VerbDescribeAccess(cx *Invocation) (any, error) {
	if err := RequireDatasetDiscoveryScope(cx.Flags); err != nil {
		return nil, err
	}
	cat, err := PickDatasets(cx.WS, cx.Flags)
	if err != nil {
		return nil, err
	}
	setID, err := cx.SetID()
	if err != nil {
		return nil, err
	}
	resolved, err := ResolveAtRevision(cx.WS, cx.Home, cat, setID, cx.Flags)
	if err != nil {
		return nil, err
	}
	pin := kchome.DatasetPin(resolved)
	if err := RequireCompleteWorkspaceRead(cx.Home, cx.Flags, pin, ""); err != nil {
		return nil, err
	}
	result, err := (discoveryservice.AccessDescribeExecutor{
		Repositories: cx.WS.Reader.Lookup(cat.Require),
		Plugins:      cx.WS.Plugins,
	}).Execute(cx.Context, discoveryservice.AccessDescribeRequest{Pin: pin})
	if err != nil {
		return nil, err
	}
	plugins, err := cx.WS.LiveAccess.Overview(pin)
	if err != nil {
		return nil, err
	}
	result.Plugins = append(result.Plugins, plugins...)
	return result, nil
}

// ResourceLookup uses an explicit component port or the Repository-selected
// LiveAccess registry. A static read needs no external lookup.
func ResourceLookup(cx *Invocation) (objectserving.StateLookup, error) {
	if cx.State != nil {
		return cx.State, nil
	}
	if cx.WS != nil && cx.WS.LiveAccess != nil {
		return cx.WS.LiveAccess, nil
	}
	return nil, nil
}
