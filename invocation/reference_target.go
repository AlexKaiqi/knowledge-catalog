package invocation

import (
	"kc/invoke"
	"kc/kernel"
	objectmodel "kc/object"
	objectservice "kc/object/service"
)

const referenceCommitFlag = "_object-reference-commit"
const referenceBindingFlag = "_object-reference-binding"

// The private value proves that coordinates were derived inside the service,
// rather than supplied as an alternate public selection.
type referenceBinding struct {
	reference objectmodel.Reference
	dataset   bool
}

// NormalizeReferenceFlags binds source identity before authorization. The
// fragment remains request data; Dataset scope never replaces source identity.
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
	workspace := ServingWorkspace(flags)
	if bound, ok := flags[referenceBindingFlag].(referenceBinding); ok {
		expectedRepo := string(ref.Repository)
		if workspace {
			expectedRepo = ""
		}
		if bound.reference != ref || bound.dataset != workspace || invoke.FlagString(flags, "repo") != expectedRepo || invoke.FlagString(flags, "object") != string(ref.Object) {
			return kernel.Fail(kernel.ErrUsageInvalid, "bound reference coordinates changed")
		}
		return nil
	}
	for _, name := range []string{"repo", "object"} {
		if _, supplied := flags[name]; supplied {
			return kernel.Fail(kernel.ErrUsageInvalid, "reference already identifies the source object; do not add --%s", name)
		}
	}
	flags["object"] = string(ref.Object)
	if workspace {
		if invoke.FlagString(flags, "ref") != "" {
			return kernel.Fail(kernel.ErrUsageInvalid, "Dataset reference reads use the accepted source commit, not a source ref")
		}
		if commit := invoke.FlagString(flags, "commit"); commit != "" {
			flags[referenceCommitFlag] = commit
		}
		delete(flags, "repo")
		delete(flags, "commit")
	} else {
		flags["repo"] = string(ref.Repository)
	}
	flags[referenceBindingFlag] = referenceBinding{reference: ref, dataset: workspace}
	return nil
}

func referenceFromFlags(flags map[string]invoke.FlagValue) (*objectmodel.Reference, error) {
	if raw := invoke.FlagString(flags, "reference"); raw != "" {
		ref, err := objectmodel.ParseReference(raw)
		return &ref, err
	}
	return nil, nil
}

func readDatasetReference(cx *Invocation, ref objectmodel.Reference) ([]objectmodel.ObservedValue, error) {
	serving, _, err := OpenServingAt(cx, cx.WS, cx.Flags)
	if err != nil {
		return nil, err
	}
	commit, selected := serving.Pin().Repositories[ref.Repository]
	if !selected {
		return nil, kernel.Fail(kernel.ErrKnowledgeRefUnresolved, "reference source is outside the Dataset publication")
	}
	if expected := cx.Flag(referenceCommitFlag); expected != "" && expected != string(commit) {
		return nil, kernel.Fail(kernel.ErrUsageInvalid, "reference source commit conflicts with the Dataset publication")
	}
	if !AllowedRepoRead(cx.Home, cx.Flags, string(ref.Repository), string(ref.Object)) {
		return nil, kernel.Fail(kernel.ErrForbidden, "reference content is not authorized")
	}
	logical, err := LogicalWorkspaceServing(cx, serving)
	if err != nil {
		return nil, err
	}
	return logical.ReadReference(cx.Context, ref)
}

func maintainReference(cx *Invocation, operation objectmodel.OpKind, value any) (any, error) {
	if ServingWorkspace(cx.Flags) || invoke.FlagBool(cx.Flags, "bulk") {
		return nil, kernel.Fail(kernel.ErrUsageInvalid, "single object maintenance requires one source Repository")
	}
	ref, err := objectmodel.ParseReference(cx.Flag("reference"))
	if err != nil {
		return nil, err
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
			return cx.WS.Reader.Require(id, kernel.ErrKnowledgeRefUnresolved)
		},
	}).Execute(cx.Context, operation, objectservice.MaintenanceRequest{
		Reference: cx.Flag("reference"), CommandID: cx.Flag("command-id"), Ref: cx.Flag("ref"),
		Base: kernel.CommitID(cx.Flag("base")), Expected: kernel.CommitID(cx.Flag("expected")),
		Value: value, Kind: objectmodel.AddressKind(cx.Flag("kind")), SchemaRef: op.SchemaRef, PathHint: op.PathHint, ValueSource: op.ValueSource,
		IfAbsent: invoke.FlagBool(cx.Flags, "if-absent"), IfDigest: kernel.Digest(cx.Flag("if-digest")), Message: cx.Flag("message"), Provenance: provenance,
	})
}
