package service

import (
	"context"
	"strings"

	"kc/kernel"
	objectmodel "kc/object"
	"kc/object/writer"
	"kc/snapshot"
)

// MaintenanceRequest uses the same reference as READ plus the Repository the
// write targets: the URN names the object, never where its Aspect is stored.
// Kind only declares a new maintenance model; it cannot reinterpret an
// existing unit or a field.
type MaintenanceRequest struct {
	Reference   string
	Repository  kernel.RepositoryID
	CommandID   string
	Ref         string
	Base        kernel.CommitID
	Expected    kernel.CommitID
	Value       any
	Kind        objectmodel.AddressKind
	SchemaRef   string
	ValueSource *objectmodel.ValueSource
	PathHint    string
	IfAbsent    bool
	IfDigest    kernel.Digest
	Message     string
	Provenance  *objectmodel.ProvenanceEnvelope
}

type MaintenanceExecutor struct {
	Repository func(kernel.RepositoryID) (objectmodel.Repository, error)
	Writer     Committer
}

func (e MaintenanceExecutor) Execute(ctx context.Context, operation objectmodel.OpKind, request MaintenanceRequest) (writer.CommitReceipt, error) {
	if e.Repository == nil || e.Writer == nil {
		return writer.CommitReceipt{}, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "object maintenance service is unavailable")
	}
	if operation != objectmodel.OpPut && operation != objectmodel.OpRemove || strings.TrimSpace(request.CommandID) == "" {
		return writer.CommitReceipt{}, kernel.Fail(kernel.ErrUsageInvalid, "maintenance requires PUT or REMOVE and a command identity")
	}
	if request.IfAbsent && request.IfDigest != "" {
		return writer.CommitReceipt{}, kernel.Fail(kernel.ErrUsageInvalid, "maintenance accepts one content precondition")
	}
	ref, err := objectmodel.ParseReference(request.Reference)
	if err != nil {
		return writer.CommitReceipt{}, err
	}
	if request.Repository == "" {
		return writer.CommitReceipt{}, kernel.Fail(kernel.ErrUsageInvalid, "object maintenance writes one Repository; name it")
	}
	ref.Repository = request.Repository
	repo, err := e.Repository(ref.Repository)
	if err != nil {
		return writer.CommitReceipt{}, err
	}
	base := request.Base
	if ledger, ok := e.Writer.(interface {
		Lookup(string) (writer.IdempotencyEntry, bool)
	}); ok {
		if prior, exists := ledger.Lookup(request.CommandID); exists {
			if prior.Request.RepositoryID != ref.Repository || prior.Request.TargetRef != snapshot.RefOrDefault(request.Ref) || base != "" && base != prior.Request.BaseCommit || request.Expected != "" && request.Expected != prior.Request.ExpectedTargetCommit {
				return writer.CommitReceipt{}, kernel.Fail(kernel.ErrIdempotencyConflict, "command identity already has a different write basis")
			}
			base = prior.Request.BaseCommit
		}
	}
	if base == "" {
		base = request.Expected
	}
	if base == "" {
		base, err = repo.Head(snapshot.RefOrDefault(request.Ref))
		if err != nil {
			return writer.CommitReceipt{}, err
		}
	}
	units, err := maintenanceUnits(repo, ref.Object, base)
	if err != nil {
		return writer.CommitReceipt{}, err
	}
	address, err := ResolveMaintenanceAddress(ref, request.Kind, units)
	if err != nil {
		return writer.CommitReceipt{}, err
	}
	op := objectmodel.Operation{Op: operation, Address: address, Value: request.Value, SchemaRef: request.SchemaRef, PathHint: request.PathHint, ValueSource: request.ValueSource}
	if request.IfAbsent {
		op.Precondition = &objectmodel.Precondition{Type: objectmodel.IfAbsent}
	} else if request.IfDigest != "" {
		op.Precondition = &objectmodel.Precondition{Type: objectmodel.IfDigestEquals, Digest: request.IfDigest}
	}
	expected := request.Expected
	if expected == "" {
		expected = base
	}
	return (CommitExecutor{Writer: e.Writer}).Execute(ctx, CommitRequest{CommandID: request.CommandID, Intent: writer.CommitIntent{
		TargetRepository: ref.Repository, TargetRef: snapshot.RefOrDefault(request.Ref),
		BaseCommit: base, ExpectedTargetCommit: expected, Operations: []objectmodel.Operation{op},
		Message: request.Message, Provenance: request.Provenance,
	}})
}

func maintenanceUnits(repo objectmodel.Repository, id objectmodel.ObjectID, base kernel.CommitID) ([]objectmodel.Address, error) {
	value, err := repo.Read(id, base)
	if kernel.CodeOf(err) == kernel.ErrObjectRefUnresolved {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if len(value.Units) > 0 {
		return value.Units, nil
	}
	if len(value.Declarations) > 0 {
		units := make([]objectmodel.Address, 0, len(value.Declarations))
		for _, declaration := range value.Declarations {
			units = append(units, declaration.Address)
		}
		return units, nil
	}
	return []objectmodel.Address{value.Address}, nil
}

// ResolveMaintenanceAddress uses fixed maintenance coordinates, never body
// shape or path depth alone, so authorization and execution share one model.
func ResolveMaintenanceAddress(ref objectmodel.Reference, kind objectmodel.AddressKind, units []objectmodel.Address) (objectmodel.Address, error) {
	tokens, err := objectmodel.PointerTokens(ref.Locator)
	if err != nil {
		return objectmodel.Address{}, err
	}
	address := objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: ref.Object}
	if len(tokens) == 0 {
		for _, unit := range units {
			if objectmodel.IsEntityBlob(unit) {
				address.Kind = unit.Kind
			}
		}
		if kind != "" && len(units) > 0 && kind != address.Kind {
			return address, unsupportedMaintenance("the declared kind conflicts with the existing root maintenance model")
		}
		if kind != "" && len(units) == 0 {
			address.Kind = kind
		}
		return address, objectmodel.AssertWritable(address)
	}
	if len(tokens) > 2 {
		return address, unsupportedMaintenance("deep fields require replacement of their complete maintenance unit")
	}
	address.AspectName = tokens[0]
	hasAspect, hasMembers := false, false
	for _, unit := range units {
		if objectmodel.IsEntityBlob(unit) {
			return address, unsupportedMaintenance("this object is maintained as one complete value")
		}
		if unit.AspectName != tokens[0] {
			continue
		}
		if unit.MemberKey == "" {
			hasAspect = true
			if len(tokens) == 1 {
				address.Kind = unit.Kind
			}
		} else {
			hasMembers = true
		}
	}
	if len(tokens) == 1 {
		if hasMembers {
			return address, unsupportedMaintenance("a Members collection is not one complete Aspect maintenance unit")
		}
		if !hasAspect {
			address.Kind = objectmodel.KindAspect
		}
	} else {
		if hasAspect {
			return address, unsupportedMaintenance("this Aspect is maintained as one complete value")
		}
		if !hasMembers && kind != objectmodel.KindMember {
			return address, unsupportedMaintenance("a new Member collection requires an explicit Member kind")
		}
		address.Kind, address.MemberKey = objectmodel.KindMember, tokens[1]
	}
	if kind != "" && kind != address.Kind {
		if len(tokens) == 1 && !hasAspect && kind == objectmodel.KindRecord {
			address.Kind = kind
		} else {
			return address, unsupportedMaintenance("the declared kind conflicts with the maintenance model")
		}
	}
	return address, objectmodel.AssertWritable(address)
}

func unsupportedMaintenance(message string) error {
	return kernel.Fail(kernel.ErrUsageInvalid, "locator is not a supported maintenance target: %s", message)
}
