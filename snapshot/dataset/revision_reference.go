package dataset

import (
	"context"
	"encoding/json"
	"strings"

	"kc/kernel"
	"kc/snapshot"
)

// RevisionReference is an accepted consumer-owned reference. Its opaque value
// is not Dataset content, user metadata, a permission, or a new revision.
// The receipt ID identifies one attempt, including after a lost commit reply.
type RevisionReference struct {
	Version int `json:"version"`
	PublicationLookup
	Owner      string               `json:"owner"`
	ID         string               `json:"id"`
	Definition kernel.Digest        `json:"definition"`
	Authority  PublicationAuthority `json:"authority"`
	Value      []byte               `json:"value"`
}

// RevisionReferenceCheckpoint fixes the actual authority version before an
// external consumer prepares a replacement. Commit never rebases this version.
type RevisionReferenceCheckpoint struct {
	PublicationAdmission
	PublicationLookup
	Owner      string
	Definition kernel.Digest
	Current    *RevisionReference
}

// RevisionReferenceCoordinator coordinates derived consumer references without
// interpreting them or changing frozen Dataset definitions. External resources
// must be ready and protected before CommitRevisionReference. Unknown outcomes
// require receipt recovery or an actual authority fence, never timeout release.
type RevisionReferenceCoordinator interface {
	PrepareRevisionReference(context.Context, PublicationLookup, string) (RevisionReferenceCheckpoint, error)
	CommitRevisionReference(context.Context, RevisionReferenceCheckpoint, []byte) (RevisionReference, error)
	RevisionReference(context.Context, PublicationLookup, string) (RevisionReference, error)
	RevisionReferenceReceipt(context.Context, string) (RevisionReference, error)
}

func validReferenceKey(lookup PublicationLookup, owner string) error {
	if lookup.SetID == "" || lookup.Revision <= 0 || owner == "" || strings.TrimSpace(owner) != owner || strings.ContainsAny(owner, "\x00\r\n") {
		return kernel.Fail(kernel.ErrUsageInvalid, "revision reference requires a Dataset, positive revision and consumer owner")
	}
	return nil
}

func referenceTarget(registry *registry, head kernel.CommitID, lookup PublicationLookup, currentOnly bool) (State, Dataset, error) {
	state, err := registry.loadAt(string(head))
	if err != nil {
		return State{}, Dataset{}, err
	}
	if state.AuthorityID != registry.AuthorityID() {
		return State{}, Dataset{}, kernel.Fail(kernel.ErrPreconditionFailed, "revision reference authority state is invalid")
	}
	accepted := newAcceptedState(state, string(head))
	current, ok := accepted.datasets[lookup.SetID]
	if !ok || current.Retired {
		code := kernel.ErrDatasetInvalid
		if currentOnly {
			code = kernel.ErrPreconditionFailed
		}
		return State{}, Dataset{}, kernel.Fail(code, "revision reference target is absent or retired")
	}
	if currentOnly && current.Revision != lookup.Revision {
		return State{}, Dataset{}, kernel.Fail(kernel.ErrPreconditionFailed, "only the current Dataset revision may replace a consumer reference")
	}
	definition, ok := accepted.versions[lookup.SetID][lookup.Revision]
	if !ok {
		return State{}, Dataset{}, kernel.Fail(kernel.ErrDatasetInvalid, "revision reference target version is unavailable")
	}
	return state, definition, nil
}

func (c *service) PrepareRevisionReference(ctx context.Context, lookup PublicationLookup, owner string) (RevisionReferenceCheckpoint, error) {
	if err := c.ensureWritable(); err != nil {
		return RevisionReferenceCheckpoint{}, err
	}
	if err := validReferenceKey(lookup, owner); err != nil {
		return RevisionReferenceCheckpoint{}, err
	}
	io, head, err := c.referenceIO(ctx)
	if err != nil {
		return RevisionReferenceCheckpoint{}, err
	}
	_, definition, err := referenceTarget(io, head, lookup, true)
	if err != nil {
		return RevisionReferenceCheckpoint{}, err
	}
	admission, err := io.publicationAdmission(string(head))
	if err != nil {
		return RevisionReferenceCheckpoint{}, err
	}
	checkpoint := RevisionReferenceCheckpoint{PublicationAdmission: admission, PublicationLookup: lookup, Owner: owner, Definition: kernel.CanonicalDigest(definition)}
	record, err := io.readRevisionReference(head, revisionReferencePath(lookup, owner))
	if kernel.CodeOf(err) == kernel.ErrObjectRefUnresolved {
		return checkpoint, nil
	}
	if err != nil {
		return RevisionReferenceCheckpoint{}, err
	}
	if record.PublicationLookup != lookup || record.Owner != owner || record.Definition != checkpoint.Definition {
		return RevisionReferenceCheckpoint{}, kernel.Fail(kernel.ErrPreconditionFailed, "consumer reference differs from fixed Dataset")
	}
	checkpoint.Current = &record
	return checkpoint, nil
}

func (c *service) CommitRevisionReference(ctx context.Context, checkpoint RevisionReferenceCheckpoint, value []byte) (result RevisionReference, err error) {
	defer func() {
		err = c.note("dataset-revision-reference", map[string]any{"setId": checkpoint.SetID, "revision": checkpoint.Revision, "owner": checkpoint.Owner, "referenceId": checkpoint.ID}, err)
		if err != nil {
			result = RevisionReference{}
		}
	}()
	if err := c.ensureWritable(); err != nil {
		return RevisionReference{}, err
	}
	record := RevisionReference{Version: 1, PublicationLookup: checkpoint.PublicationLookup, Owner: checkpoint.Owner, ID: checkpoint.ID, Definition: checkpoint.Definition, Authority: checkpoint.PublicationAuthority, Value: append([]byte(nil), value...)}
	if err := validateRevisionReference(record); err != nil {
		return RevisionReference{}, err
	}
	if ctx == nil {
		ctx = context.Background()
	}
	unlock, err := lockForContext(ctx, &c.writeMu)
	if err != nil {
		return RevisionReference{}, err
	}
	defer unlock()
	unlockRegistry, err := lockForContext(ctx, &c.registry.writeMu)
	if err != nil {
		return RevisionReference{}, err
	}
	defer unlockRegistry()
	io, err := c.registry.boundIO(ctx)
	if err != nil {
		return RevisionReference{}, err
	}
	actual, err := io.publicationAuthority(string(checkpoint.Head))
	if err != nil {
		return RevisionReference{}, err
	}
	if actual != checkpoint.PublicationAuthority {
		return RevisionReference{}, kernel.Fail(kernel.ErrPreconditionFailed, "revision reference authority binding changed")
	}
	state, definition, err := referenceTarget(io, checkpoint.Head, checkpoint.PublicationLookup, true)
	if err != nil {
		return RevisionReference{}, err
	}
	if kernel.CanonicalDigest(definition) != checkpoint.Definition {
		return RevisionReference{}, kernel.Fail(kernel.ErrPreconditionFailed, "revision reference definition changed")
	}
	// An old attempt is never promoted to a new head, even for equal values.
	if _, err := io.readRevisionReference(checkpoint.Head, revisionReferenceReceiptPath(checkpoint.ID)); kernel.CodeOf(err) != kernel.ErrObjectRefUnresolved {
		if err != nil {
			return RevisionReference{}, err
		}
		return RevisionReference{}, kernel.Fail(kernel.ErrPreconditionFailed, "revision reference attempt already exists at its base")
	}
	body, err := json.Marshal(record)
	if err != nil {
		return RevisionReference{}, err
	}
	if err := ctx.Err(); err != nil {
		return RevisionReference{}, err
	}
	as, requestID, ruleID, _ := c.requestContext()
	next, err := io.tree.ApplyTreeCommit(snapshot.TreeChangeSet{TargetRepository: io.authority.ID(), TargetRef: io.ref, BaseCommit: checkpoint.Head, ExpectedTargetCommit: checkpoint.Head,
		Changes: []snapshot.TreeChange{{Path: revisionReferencePath(record.PublicationLookup, record.Owner), Content: body}, {Path: revisionReferenceReceiptPath(record.ID), Content: body}},
		Message: "dataset-revision-reference " + record.Owner, Author: as, RequestID: requestID, RuleID: ruleID})
	if err != nil {
		return RevisionReference{}, err
	}
	if next == "" || next == checkpoint.Head {
		return RevisionReference{}, kernel.Fail(kernel.ErrTemporaryUnavailable, "revision reference commit was not confirmed")
	}
	c.registry.accept(string(next))
	c.current.Store(newAcceptedState(state, string(next)))
	return record, nil
}
