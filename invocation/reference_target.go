package invocation

import (
	"sort"

	"kc/invoke"
	"kc/kernel"
	objectmodel "kc/object"
	objectservice "kc/object/service"
	"kc/snapshot"
)

const referenceCommitFlag = "_object-reference-commit"
const referenceBindingFlag = "_object-reference-binding"
const referenceHoldingsFlag = "_object-reference-holdings"
const referencePlannedFlag = "_object-reference-planned"

// The private value proves that coordinates were derived inside the service,
// rather than supplied as an alternate public selection. Its reference carries
// the Repository the scope resolved to; the public URN never does.
type referenceBinding struct {
	reference objectmodel.Reference
	dataset   bool
}

// NormalizeReferenceFlags parses the object URN and records the request scope.
// The URN names only the object; --repo optionally narrows the scope to one
// Repository and --dataset selects a Dataset. The two scopes are exclusive.
func NormalizeReferenceFlags(flags map[string]invoke.FlagValue) error {
	raw := invoke.FlagString(flags, "reference")
	if raw == "" {
		return nil
	}
	ref, err := objectmodel.ParseReference(raw)
	if err != nil {
		return err
	}
	for _, name := range []string{"aspect", "member", "include", "exclude"} {
		if _, supplied := flags[name]; supplied {
			return kernel.Fail(kernel.ErrUsageInvalid, "reference already selects content; do not add --%s", name)
		}
	}
	if ref.Locator != "" && invoke.FlagString(flags, "shape") != "" {
		return kernel.Fail(kernel.ErrUsageInvalid, "reference already selects content; do not add --shape")
	}
	workspace := ServingWorkspace(flags)
	if bound, ok := flags[referenceBindingFlag].(referenceBinding); ok {
		if bound.reference.Object != ref.Object || bound.reference.Locator != ref.Locator || bound.dataset != workspace ||
			invoke.FlagString(flags, "repo") != string(bound.reference.Repository) || invoke.FlagString(flags, "object") != string(ref.Object) {
			return kernel.Fail(kernel.ErrUsageInvalid, "bound reference coordinates changed")
		}
		return nil
	}
	if _, forged := flags[referenceBindingFlag]; forged {
		return kernel.Fail(kernel.ErrUsageInvalid, "reference binding is internal; supply only the object URN and its scope")
	}
	if _, supplied := flags["object"]; supplied {
		return kernel.Fail(kernel.ErrUsageInvalid, "reference already identifies the object; do not add --object")
	}
	flags["object"] = string(ref.Object)
	if workspace {
		if invoke.FlagString(flags, "repo") != "" {
			return kernel.Fail(kernel.ErrUsageInvalid, "choose one scope: --dataset or --repo")
		}
		if invoke.FlagString(flags, "ref") != "" {
			return kernel.Fail(kernel.ErrUsageInvalid, "Dataset reference reads use the accepted source commit, not a source ref")
		}
		if commit := invoke.FlagString(flags, "commit"); commit != "" {
			flags[referenceCommitFlag] = commit
		}
		delete(flags, "repo")
		delete(flags, "commit")
	} else {
		if invoke.FlagString(flags, "repo") == "" && invoke.FlagString(flags, "commit") != "" {
			return kernel.Fail(kernel.ErrUsageInvalid, "--commit belongs to one Repository; add --repo")
		}
		ref.Repository = kernel.RepositoryID(invoke.FlagString(flags, "repo"))
	}
	flags[referenceBindingFlag] = referenceBinding{reference: ref, dataset: workspace}
	return nil
}

// referenceFromFlags returns the bound reference, including the Repository the
// scope resolved to, or the parsed URN before binding.
func referenceFromFlags(flags map[string]invoke.FlagValue) (*objectmodel.Reference, error) {
	raw := invoke.FlagString(flags, "reference")
	if raw == "" {
		return nil, nil
	}
	if bound, ok := flags[referenceBindingFlag].(referenceBinding); ok {
		ref := bound.reference
		return &ref, nil
	}
	ref, err := objectmodel.ParseReference(raw)
	return &ref, err
}

func rebindReference(flags map[string]invoke.FlagValue, repository kernel.RepositoryID) {
	bound := flags[referenceBindingFlag].(referenceBinding)
	bound.reference.Repository = repository
	flags[referenceBindingFlag] = bound
	flags["repo"] = string(repository)
}

// bindReferenceScope resolves an object URN inside the deployment scope when
// the caller did not name a Repository: every Repository holding the object
// contributes its Aspects, the same Aspect from two Repositories is a conflict,
// and holders the caller cannot read are withheld rather than hidden. Writes
// always name their Repository; they never guess where an Aspect belongs.
func bindReferenceScope(cx *Invocation, action string) error {
	bound, ok := cx.Flags[referenceBindingFlag].(referenceBinding)
	if !ok || bound.dataset || bound.reference.Repository != "" {
		return nil
	}
	if !TypedInvocationReadOnly(action) {
		return kernel.Fail(kernel.ErrUsageInvalid, "object maintenance writes one Repository; name it with --repo")
	}
	ref := snapshot.RefOrDefault(cx.Flag("ref"))
	candidates := map[kernel.RepositoryID]kernel.CommitID{}
	for _, id := range cx.WS.Store.IDs() {
		if string(id) == cx.WS.File.Registry.ID {
			continue
		}
		store, _ := cx.WS.Store.Get(id)
		if commit, found := store.GetRef(ref); found {
			candidates[id] = commit
		}
	}
	if err := requireReadableScope(cx, bound.reference, candidates); err != nil {
		return err
	}
	planned, err := planReferenceHoldings(bound.reference, candidates, cx.WS.Reader.PlanReference)
	if err != nil {
		return err
	}
	selection, err := planned.selectFor(cx, bound.reference)
	if invoke.FlagString(cx.Flags, "shape") != "" {
		// A shape decides conflicts and withheld content per selection; the
		// root plan only binds authorization to one readable holder.
		holder, ok := planned.firstReadable(cx, bound.reference)
		if !ok {
			return err
		}
		cx.Flags[referencePlannedFlag] = planned
		rebindReference(cx.Flags, holder.Repository)
		cx.Flags["commit"] = string(holder.Commit)
		return nil
	}
	if err != nil {
		return err
	}
	rebindReference(cx.Flags, selection.Selected[0].Repository)
	cx.Flags["commit"] = string(selection.Selected[0].Commit)
	cx.Flags[referenceHoldingsFlag] = selection
	return nil
}

// plannedHoldings is the metadata-only root plan of every candidate
// Repository. Readability depends on the selected Aspect, so it is decided
// when one reference is selected.
type plannedHoldings struct {
	ids        []kernel.RepositoryID
	candidates map[kernel.RepositoryID]kernel.CommitID
	units      map[kernel.RepositoryID][]objectmodel.Address
}

func planReferenceHoldings(reference objectmodel.Reference, candidates map[kernel.RepositoryID]kernel.CommitID, plan func(objectmodel.Reference, kernel.CommitID) ([]objectmodel.Address, error)) (plannedHoldings, error) {
	planned := plannedHoldings{candidates: candidates, units: map[kernel.RepositoryID][]objectmodel.Address{}}
	for id := range candidates {
		planned.ids = append(planned.ids, id)
	}
	sort.Slice(planned.ids, func(i, j int) bool { return planned.ids[i] < planned.ids[j] })
	for _, id := range planned.ids {
		units, err := plan(objectmodel.Reference{Repository: id, Object: reference.Object}, candidates[id])
		if kernel.CodeOf(err) == kernel.ErrObjectRefUnresolved || err == nil && len(units) == 0 {
			continue
		}
		if err != nil {
			return plannedHoldings{}, err
		}
		planned.units[id] = units
	}
	return planned, nil
}

// selectFor applies readability for the reference's Aspect and the identity
// rule (conflict, withheld) to the planned holdings.
func (p plannedHoldings) selectFor(cx *Invocation, reference objectmodel.Reference) (objectmodel.HoldingSelection, error) {
	aspect := referenceAspect(reference)
	readable := map[kernel.RepositoryID]bool{}
	for _, id := range p.ids {
		readable[id] = AllowedRepoAspectRead(cx.Home, cx.Flags, string(id), string(reference.Object), aspect)
	}
	if !anyTrue(readable) {
		return objectmodel.HoldingSelection{}, kernel.Fail(kernel.ErrForbidden, "the caller may not read any Repository in the request scope")
	}
	holdings := make([]objectmodel.Holding, 0, len(p.units))
	for _, id := range p.ids {
		if units, ok := p.units[id]; ok {
			holdings = append(holdings, objectmodel.HoldingOf(id, p.candidates[id], readable[id], units))
		}
	}
	return objectmodel.SelectHoldings(reference.Object, reference.Locator, holdings)
}

func (p plannedHoldings) firstReadable(cx *Invocation, reference objectmodel.Reference) (objectmodel.Holding, bool) {
	for _, id := range p.ids {
		if _, held := p.units[id]; held && AllowedRepoRead(cx.Home, cx.Flags, string(id), string(reference.Object)) {
			return objectmodel.Holding{Repository: id, Commit: p.candidates[id]}, true
		}
	}
	return objectmodel.Holding{}, false
}

func referenceAspect(reference objectmodel.Reference) string {
	if tokens, err := objectmodel.PointerTokens(reference.Locator); err == nil && len(tokens) > 0 {
		return tokens[0]
	}
	return ""
}

// selectReferenceHoldings plans the root object in every candidate Repository
// at its fixed commit (metadata only) and applies the object identity rule.
func selectReferenceHoldings(cx *Invocation, reference objectmodel.Reference, candidates map[kernel.RepositoryID]kernel.CommitID, plan func(objectmodel.Reference, kernel.CommitID) ([]objectmodel.Address, error)) (objectmodel.HoldingSelection, error) {
	if err := requireReadableScope(cx, reference, candidates); err != nil {
		return objectmodel.HoldingSelection{}, err
	}
	planned, err := planReferenceHoldings(reference, candidates, plan)
	if err != nil {
		return objectmodel.HoldingSelection{}, err
	}
	return planned.selectFor(cx, reference)
}

// requireReadableScope fails before any planning when the caller may read no
// candidate Repository for the referenced Aspect.
func requireReadableScope(cx *Invocation, reference objectmodel.Reference, candidates map[kernel.RepositoryID]kernel.CommitID) error {
	aspect := referenceAspect(reference)
	for id := range candidates {
		if AllowedRepoAspectRead(cx.Home, cx.Flags, string(id), string(reference.Object), aspect) {
			return nil
		}
	}
	return kernel.Fail(kernel.ErrForbidden, "the caller may not read any Repository in the request scope")
}

func anyTrue(values map[kernel.RepositoryID]bool) bool {
	for _, value := range values {
		if value {
			return true
		}
	}
	return false
}

func readDatasetReference(cx *Invocation, ref objectmodel.Reference) ([]objectmodel.ObservedValue, error) {
	serving, _, err := OpenServingAt(cx, cx.WS, cx.Flags)
	if err != nil {
		return nil, err
	}
	pin := serving.Pin().Repositories
	selection, err := selectReferenceHoldings(cx, ref, pin, func(reference objectmodel.Reference, commit kernel.CommitID) ([]objectmodel.Address, error) {
		member, err := serving.Member(reference.Repository)
		if err != nil {
			return nil, err
		}
		planner, ok := member.(objectmodel.ReferencePlanner)
		if !ok {
			return nil, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "dataset member has no metadata-only reference planner")
		}
		return planner.PlanReference(reference, commit)
	})
	if err != nil {
		return nil, err
	}
	if expected := cx.Flag(referenceCommitFlag); expected != "" {
		for _, h := range selection.Selected {
			if expected != string(h.Commit) {
				return nil, kernel.Fail(kernel.ErrUsageInvalid, "reference source commit conflicts with the Dataset publication")
			}
		}
	}
	logical, err := LogicalWorkspaceServing(cx, serving)
	if err != nil {
		return nil, err
	}
	values := make([]objectmodel.ObservedValue, 0, len(selection.Selected))
	for _, h := range selection.Selected {
		member := ref
		member.Repository = h.Repository
		read, err := logical.ReadReference(cx.Context, member)
		if err != nil {
			return nil, err
		}
		values = append(values, read...)
	}
	if len(values) <= 1 {
		for i := range values {
			values[i].Withheld = selection.Withheld
		}
		return values, nil
	}
	composed, err := composeObserved(ref, values)
	if err != nil {
		return nil, err
	}
	composed.Withheld = selection.Withheld
	return []objectmodel.ObservedValue{composed}, nil
}

func composeObserved(ref objectmodel.Reference, values []objectmodel.ObservedValue) (objectmodel.ObservedValue, error) {
	plain := make([]objectmodel.ObjectValue, 0, len(values))
	for _, value := range values {
		if len(value.Observations) > 0 || len(value.Failures) > 0 {
			return objectmodel.ObservedValue{}, kernel.Fail(kernel.ErrUsageInvalid, "observed values span Repositories; read %s with a locator that selects one Aspect", ref.Object)
		}
		plain = append(plain, observedObject(value))
	}
	raw, err := objectmodel.FormatReference(ref)
	if err != nil {
		return objectmodel.ObservedValue{}, err
	}
	composed, err := objectmodel.ComposeHoldingValues(raw, plain)
	if err != nil {
		return objectmodel.ObservedValue{}, err
	}
	return objectmodel.ObservedValue{FederatedValue: objectmodel.FederatedValue{
		Reference: composed.Reference, Locator: composed.Locator, Digest: composed.Digest,
		ObjectRef: composed.ObjectRef, ObjectID: composed.ObjectRef.Object, Address: composed.Address, Value: composed.Value,
		Units: composed.Units, Declarations: composed.Declarations, Sources: composed.Sources,
	}}, nil
}

func observedObject(observed objectmodel.ObservedValue) objectmodel.ObjectValue {
	return objectmodel.ObjectValue{
		Reference: observed.Reference, Locator: observed.Locator, Digest: observed.Digest,
		ObjectRef: observed.ObjectRef, Repository: observed.Repository, Commit: observed.Commit,
		Address: observed.Address, Value: observed.Value, Provenance: observed.Provenance,
		Units: observed.Units, Declarations: observed.Declarations,
		Sources: observed.Sources, Withheld: observed.Withheld,
	}
}

func maintainReference(cx *Invocation, operation objectmodel.OpKind, value any) (any, error) {
	if ServingWorkspace(cx.Flags) || invoke.FlagBool(cx.Flags, "bulk") {
		return nil, kernel.Fail(kernel.ErrUsageInvalid, "single object maintenance requires one source Repository")
	}
	ref, err := referenceFromFlags(cx.Flags)
	if err != nil {
		return nil, err
	}
	if ref.Repository == "" {
		return nil, kernel.Fail(kernel.ErrUsageInvalid, "object maintenance writes one Repository; name it with --repo")
	}
	if err := RequireSnapshotRepository(cx, string(ref.Repository)); err != nil {
		return nil, err
	}
	op, err := WriteOperation(cx.Flags, operation, value)
	if err != nil {
		return nil, err
	}
	provenance := OriginFrom(cx.Flags)
	if supplied, ok := cx.Flags["_object-provenance"].(*objectmodel.ProvenanceEnvelope); ok {
		provenance = supplied
	}
	SetTelemetryChangeCounts(cx.Observation, []objectmodel.Operation{op})
	return (objectservice.MaintenanceExecutor{
		Writer: cx.WS.Writer,
		Repository: func(id kernel.RepositoryID) (objectmodel.Repository, error) {
			return cx.WS.Reader.Require(id, kernel.ErrObjectRefUnresolved)
		},
	}).Execute(cx.Context, operation, objectservice.MaintenanceRequest{
		Reference: cx.Flag("reference"), Repository: ref.Repository, CommandID: cx.Flag("command-id"), Ref: cx.Flag("ref"),
		Base: kernel.CommitID(cx.Flag("base")), Expected: kernel.CommitID(cx.Flag("expected")),
		Value: value, Kind: objectmodel.AddressKind(cx.Flag("kind")), SchemaRef: op.SchemaRef, PathHint: op.PathHint, ValueSource: op.ValueSource,
		IfAbsent: invoke.FlagBool(cx.Flags, "if-absent"), IfDigest: kernel.Digest(cx.Flag("if-digest")), Message: cx.Flag("message"), Provenance: provenance,
	})
}
