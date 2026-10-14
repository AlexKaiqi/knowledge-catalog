package connector

import (
	"sort"
	"strings"

	"kc/kernel"
	objectmodel "kc/object"
	"kc/snapshot"
)

const defaultTargetRef = snapshot.DefaultRef

// PreviewResult is a ChangeSet plus counts. Empty previews must not be committed.
type PreviewResult struct {
	ChangeSet objectmodel.CommitChangeSet `json:"changeSet"`
	Summary   Summary                     `json:"summary"`
	Empty     bool                        `json:"empty"`
}

// Envelope builds SOURCE provenance. sourceRefs are required by Preview.
func Envelope(connectorID string, sourceRefs []string, producedAt string) *objectmodel.ProvenanceEnvelope {
	return &objectmodel.ProvenanceEnvelope{
		OriginKind:  objectmodel.OriginSource,
		ActorRef:    connectorID,
		ActivityRef: connectorID,
		SourceRefs:  append([]string(nil), sourceRefs...),
		ProducedAt:  producedAt,
	}
}

// CommandID is connector:<id>:<runKey>. Same content retries the same id.
func CommandID(connectorID, runKey string) string {
	return "connector:" + connectorID + ":" + runKey
}

// RunKey is a stable digest of operations, suitable as CommandID runKey.
func RunKey(ops []objectmodel.Operation) string {
	return string(kernel.CanonicalDigest(ops))
}

// Validate reports whether this Scope declares anything to own.
func (s Scope) Validate() error {
	if len(s.Aspects) == 0 && !s.AllowEntity {
		return kernel.Fail(kernel.ErrUsageInvalid, "connector scope must declare aspects or allowEntity")
	}
	for _, name := range s.Aspects {
		if strings.TrimSpace(name) == "" {
			return kernel.Fail(kernel.ErrUsageInvalid, "connector scope aspect name is empty")
		}
	}
	return nil
}

// Contains reports whether the connector may write this Address.
func (s Scope) Contains(a objectmodel.Address) bool {
	if s.ObjectPrefix != "" && !strings.HasPrefix(string(a.ObjectID), s.ObjectPrefix) {
		return false
	}
	if a.AspectName == "" {
		return s.AllowEntity
	}
	for _, name := range s.Aspects {
		if name == a.AspectName {
			return true
		}
	}
	return false
}

// Preview diffs Desired against Observed inside Scope and returns a COMMIT ChangeSet.
// It does not write. ModePatch never REMOVE; ModeReconcile REMOVE only Observed∩Scope missing from Desired.
func Preview(plan Plan) (PreviewResult, error) {
	if strings.TrimSpace(plan.ConnectorID) == "" {
		return PreviewResult{}, kernel.Fail(kernel.ErrUsageInvalid, "connector id is required")
	}
	if plan.Mode != ModePatch && plan.Mode != ModeReconcile {
		return PreviewResult{}, kernel.Fail(kernel.ErrUsageInvalid, "connector mode must be patch or reconcile")
	}
	if err := plan.Scope.Validate(); err != nil {
		return PreviewResult{}, err
	}
	if plan.TargetRepository == "" {
		return PreviewResult{}, kernel.Fail(kernel.ErrWriteTargetRequired, "write requires a target repository")
	}
	if len(plan.SourceRefs) == 0 {
		return PreviewResult{}, kernel.Fail(kernel.ErrUsageInvalid, "SOURCE provenance requires sourceRefs")
	}
	desired, err := desiredMap(plan)
	if err != nil {
		return PreviewResult{}, err
	}
	observed, ignored := observedMap(plan)
	var operations []objectmodel.Operation
	summary := Summary{Ignored: ignored}
	for key, unit := range desired {
		digest := kernel.CanonicalDigest(unit.Value)
		existing, ok := observed[key]
		if !ok {
			operations = append(operations, putOp(unit, &objectmodel.Precondition{Type: objectmodel.IfAbsent}))
			summary.Added++
			continue
		}
		declarationDigest := objectmodel.DeclarationDigest(unit.SchemaRef, unit.ValueSource)
		declarationUnchanged := existing.DeclarationDigest == declarationDigest
		if existing.DeclarationDigest == "" {
			// Backward compatibility is safe only for legacy value-only units. A
			// declared Schema or Binding must be PUT once so its declaration cannot
			// be mistaken for an unchanged nil value.
			declarationUnchanged = unit.SchemaRef == "" && unit.ValueSource.Normalized() == nil
		}
		if existing.Digest == digest && declarationUnchanged {
			summary.Unchanged++
			continue
		}
		operations = append(operations, putOp(unit, &objectmodel.Precondition{Type: objectmodel.IfDigestEquals, Digest: existing.Digest}))
		summary.Updated++
	}
	if plan.Mode == ModeReconcile {
		for key, addr := range observedAddrs(plan, observed) {
			if _, ok := desired[key]; ok {
				continue
			}
			operations = append(operations, objectmodel.Operation{
				Op:      objectmodel.OpRemove,
				Address: addr,
				Reason:  "absent-from-source",
			})
			summary.Removed++
		}
	}
	sort.Slice(operations, func(i, j int) bool {
		return objectmodel.AddressKey(operations[i].Address) < objectmodel.AddressKey(operations[j].Address)
	})
	targetRef := plan.TargetRef
	if targetRef == "" {
		targetRef = defaultTargetRef
	}
	actor := plan.ActorRef
	if actor == "" {
		actor = plan.ConnectorID
	}
	message := plan.Message
	if message == "" {
		message = "connector " + plan.ConnectorID + " " + string(plan.Mode)
	}
	envelope := Envelope(actor, plan.SourceRefs, plan.ProducedAt)
	return PreviewResult{
		Summary: summary,
		Empty:   len(operations) == 0,
		ChangeSet: objectmodel.CommitChangeSet{
			TargetRepository:     plan.TargetRepository,
			TargetRef:            targetRef,
			BaseCommit:           plan.BaseCommit,
			ExpectedTargetCommit: plan.BaseCommit,
			Operations:           operations,
			Message:              message,
			Provenance:           envelope,
		},
	}, nil
}

func desiredMap(plan Plan) (map[string]Unit, error) {
	out := map[string]Unit{}
	for _, unit := range plan.Desired {
		if err := objectmodel.AssertWritable(unit.Address); err != nil {
			return nil, err
		}
		if err := objectmodel.ValidateValueSource(unit.ValueSource); err != nil {
			return nil, err
		}
		if !plan.Scope.Contains(unit.Address) {
			return nil, kernel.Fail(kernel.ErrScopeDenied, "address %s is outside connector scope", objectmodel.AddressKey(unit.Address))
		}
		key := objectmodel.AddressKey(unit.Address)
		if _, exists := out[key]; exists {
			return nil, kernel.Fail(kernel.ErrUsageInvalid, "duplicate desired address %s", key)
		}
		out[key] = unit
	}
	return out, nil
}

func observedMap(plan Plan) (map[string]Observed, int) {
	out := map[string]Observed{}
	ignored := 0
	for _, item := range plan.Observed {
		if !plan.Scope.Contains(item.Address) {
			ignored++
			continue
		}
		out[objectmodel.AddressKey(item.Address)] = item
	}
	return out, ignored
}

func observedAddrs(plan Plan, digests map[string]Observed) map[string]objectmodel.Address {
	out := map[string]objectmodel.Address{}
	for _, item := range plan.Observed {
		key := objectmodel.AddressKey(item.Address)
		if _, ok := digests[key]; !ok {
			continue
		}
		out[key] = item.Address
	}
	return out
}

func putOp(unit Unit, pre *objectmodel.Precondition) objectmodel.Operation {
	return objectmodel.Operation{
		Op:           objectmodel.OpPut,
		Address:      unit.Address,
		Value:        unit.Value,
		PathHint:     unit.PathHint,
		SchemaRef:    unit.SchemaRef,
		ValueSource:  unit.ValueSource,
		Precondition: pre,
	}
}
