package invocation

import (
	discoveryservice "kc/discovery/service"
	kchome "kc/home"
	objectserving "kc/liveaccess"

	index "kc/discovery/projection"
	"kc/invoke"
	"kc/kernel"
	objectmodel "kc/object"

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
	return datasetProjectionOperation(cx, false)
}

func VerbIndexSync(cx *Invocation) (any, error) {
	return datasetProjectionOperation(cx, true)
}

// A selected member preserves the Dataset declaration namespace while the
// operation service uses its existing fixed-source projection contract.
type datasetProjectionMember struct{ repo objectmodel.Repository }

func (m datasetProjectionMember) Require(id kernel.RepositoryID, _ kernel.ErrorCode) (objectmodel.Repository, error) {
	if m.repo.ID() != id {
		return nil, kernel.Fail(kernel.ErrObjectRefUnresolved, "Repository is outside this Dataset member")
	}
	return m.repo, nil
}

func datasetProjectionOperation(cx *Invocation, sync bool) (any, error) {
	if err := RequireDatasetDiscoveryScope(cx.Flags); err != nil {
		return nil, err
	}
	serving, cat, err := OpenServingAt(cx, cx.WS, cx.Flags)
	if err != nil {
		return nil, err
	}
	pin := serving.Pin()
	if !sync {
		if err := RequireCompleteWorkspaceRead(cx.Home, cx.Flags, pin, ""); err != nil {
			return nil, err
		}
	}
	if cx.WS.Plugins == nil || !cx.WS.Plugins.Configured() {
		return nil, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "Dataset index plugin is not configured")
	}
	ids := make([]kernel.RepositoryID, 0, len(pin.Repositories))
	for id := range pin.Repositories {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	members := make([]map[string]any, 0, len(ids))
	for _, id := range ids {
		members = append(members, map[string]any{"repository": id, "commit": pin.Repositories[id]})
	}
	if len(ids) == 0 {
		return nil, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "Dataset has no selected index sources")
	}
	repo, err := cx.WS.Plugins.DatasetMember(cx.WS.Reader.Lookup(cat.Require), pin, ids[0])
	if err != nil {
		return nil, err
	}
	out := map[string]any{"dataset": pin.SetID, "revision": pin.Revision, "members": members}
	lookup := datasetProjectionMember{repo: repo}
	if !sync {
		description, err := (discoveryservice.ProjectionDescribeExecutor{Repositories: lookup, Projection: cx.WS.Index}).Execute(cx.Context, discoveryservice.ProjectionDescribeRequest{Repository: ids[0], Commit: pin.Repositories[ids[0]]})
		if err != nil {
			return nil, err
		}
		out["snapshot"], err = datasetProjectionFacts(description)
		return out, err
	}
	current, err := cat.Set(string(pin.SetID))
	if err != nil {
		return nil, err
	}
	var state objectserving.StateLookup
	if current.Revision == pin.Revision {
		state, err = ResourceLookup(cx)
		if err != nil {
			return nil, err
		}
	}
	// Preparation is maintenance: unit limits still apply, a read's shared
	// budget does not (doc/design/liveaccess.md: 索引由服务身份准备).
	prepare := objectserving.WithoutReadBudget(cx.Context)
	result, err := (discoveryservice.ProjectionSyncExecutor{Repositories: lookup, Projection: cx.WS.Index, Observe: func() func() { return ObserveProjectionExecution(cx) }}).Execute(prepare, discoveryservice.ProjectionSyncRequest{Repository: ids[0], Commit: pin.Repositories[ids[0]], State: state, StateRequest: func() (objectmodel.RequestContext, error) { return stateRequestContextFrom(cx) }})
	if err != nil {
		return nil, err
	}
	out["snapshot"], err = datasetProjectionFacts(result.Snapshot)
	if err != nil {
		return nil, err
	}
	if result.State != nil {
		out["state"], err = datasetProjectionFacts(result.State)
	}
	return out, err
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

func VerbIndexNotify(cx *Invocation) (any, error) {
	notice, err := ChangeNoticeFromFlags(cx.Flags)
	if err != nil {
		return nil, err
	}
	if _, err := RequireRepo(cx.WS, string(notice.Repository)); err != nil {
		return nil, err
	}
	if cx.WS.Plugins == nil {
		return nil, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "projection controller is not configured")
	}
	lookup, err := ResourceLookup(cx)
	if err != nil {
		return nil, err
	}
	request, err := stateRequestContextFrom(cx)
	if err != nil {
		return nil, err
	}
	return (discoveryservice.ProjectionNoticeExecutor{
		Repositories: cx.WS.Reader,
		Projection:   cx.WS.Index,
		Controller:   cx.WS.Plugins,
	}).Execute(cx.Context, discoveryservice.ProjectionNoticeRequest{
		Notice:  notice,
		State:   lookup,
		Request: request,
	})
}

func ChangeNoticeFromFlags(flags map[string]invoke.FlagValue) (index.ChangeNotice, error) {
	repo, err := invoke.RequireFlag(flags, "repo")
	if err != nil {
		return index.ChangeNotice{}, err
	}
	notice := index.ChangeNotice{
		Repository:     kernel.RepositoryID(repo),
		Ref:            invoke.FlagString(flags, "ref"),
		SourceRevision: invoke.FlagString(flags, "source-revision"),
	}
	if object := invoke.FlagString(flags, "object"); object != "" {
		kind := objectmodel.AddressKind(invoke.FlagString(flags, "kind"))
		if kind == "" {
			if invoke.FlagString(flags, "aspect") != "" {
				kind = objectmodel.KindAspect
			} else {
				kind = objectmodel.KindEntity
			}
		}
		notice.Address = &objectmodel.Address{
			Kind: kind, ObjectID: objectmodel.ObjectID(object), AspectName: invoke.FlagString(flags, "aspect"),
		}
	}
	return notice, index.ValidateChangeNotice(notice)
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
