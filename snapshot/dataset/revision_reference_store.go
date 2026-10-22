package dataset

import (
	"bytes"
	"context"
	"encoding/json"
	"io"

	"kc/kernel"
)

func revisionReferencePath(lookup PublicationLookup, owner string) string {
	return "_revision-references/current/" + string(kernel.CanonicalDigest([]any{lookup, owner})) + ".json"
}

func revisionReferenceReceiptPath(id string) string {
	return "_revision-references/receipts/" + id + ".json"
}

func validateRevisionReference(record RevisionReference) error {
	if err := validReferenceKey(record.PublicationLookup, record.Owner); err != nil {
		return err
	}
	if record.Version != 1 || !validAdmission(record.ID) || record.Definition == "" || record.Authority.AuthorityID == "" || record.Authority.Binding == "" || record.Authority.Ref == "" || record.Authority.Head == "" || len(record.Value) == 0 {
		return kernel.Fail(kernel.ErrPreconditionFailed, "revision reference is incomplete or unsupported")
	}
	return nil
}

func (g *registry) readRevisionReference(head kernel.CommitID, path string) (RevisionReference, error) {
	raw, err := g.tree.ReadFile(path, head)
	if err != nil {
		return RevisionReference{}, err
	}
	var record RevisionReference
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&record); err != nil {
		return RevisionReference{}, kernel.Fail(kernel.ErrPreconditionFailed, "revision reference is corrupt: %v", err)
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return RevisionReference{}, kernel.Fail(kernel.ErrPreconditionFailed, "revision reference has trailing content")
	}
	if err := validateRevisionReference(record); err != nil {
		return RevisionReference{}, err
	}
	actual, err := g.publicationAuthority(string(record.Authority.Head))
	if err != nil {
		return RevisionReference{}, err
	}
	if actual != record.Authority {
		return RevisionReference{}, kernel.Fail(kernel.ErrPreconditionFailed, "revision reference belongs to another authority binding")
	}
	return record, nil
}

func (c *service) referenceIO(ctx context.Context) (*registry, kernel.CommitID, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	registry, err := c.registry.boundIO(ctx)
	if err != nil {
		return nil, "", err
	}
	head, err := registry.authority.Head(registry.ref)
	if err != nil {
		return nil, "", err
	}
	if head == "" {
		return nil, "", kernel.Fail(kernel.ErrVersionUnresolved, "revision reference authority is unavailable")
	}
	return registry, head, nil
}

// RevisionReference resolves the last accepted reference for a readable Dataset
// revision at the actual authority, independently of this process's cache.
func (c *service) RevisionReference(ctx context.Context, lookup PublicationLookup, owner string) (record RevisionReference, err error) {
	defer c.finishReferenceRead(&record, &err)
	ctx, done := c.referenceContext(ctx)
	defer done()
	if err := ctx.Err(); err != nil {
		return RevisionReference{}, err
	}

	if err := validReferenceKey(lookup, owner); err != nil {
		return RevisionReference{}, err
	}
	registry, head, err := c.referenceIO(ctx)
	if err != nil {
		return RevisionReference{}, err
	}
	_, definition, err := referenceTarget(registry, head, lookup, false)
	if err != nil {
		return RevisionReference{}, err
	}
	record, err = registry.readRevisionReference(head, revisionReferencePath(lookup, owner))
	if err != nil {
		return RevisionReference{}, err
	}
	if record.PublicationLookup != lookup || record.Owner != owner || record.Definition != kernel.CanonicalDigest(definition) {
		return RevisionReference{}, kernel.Fail(kernel.ErrPreconditionFailed, "revision reference has different immutable coordinates")
	}
	return record, nil
}

// Receipt lookup reports acceptance, including after replacement or retirement;
// it does not establish that the reference remains a live consumer root.
func (c *service) RevisionReferenceReceipt(ctx context.Context, id string) (record RevisionReference, err error) {
	defer c.finishReferenceRead(&record, &err)
	ctx, done := c.referenceContext(ctx)
	defer done()
	if err := ctx.Err(); err != nil {
		return RevisionReference{}, err
	}

	if !validAdmission(id) {
		return RevisionReference{}, kernel.Fail(kernel.ErrUsageInvalid, "revision reference receipt requires a valid attempt id")
	}
	registry, head, err := c.referenceIO(ctx)
	if err != nil {
		return RevisionReference{}, err
	}
	state, err := registry.loadAt(string(head))
	if err != nil {
		return RevisionReference{}, err
	}
	if state.AuthorityID != registry.AuthorityID() {
		return RevisionReference{}, kernel.Fail(kernel.ErrPreconditionFailed, "revision reference receipt authority is invalid")
	}
	record, err = registry.readRevisionReference(head, revisionReferenceReceiptPath(id))
	if err != nil {
		return RevisionReference{}, err
	}
	if record.ID != id {
		return RevisionReference{}, kernel.Fail(kernel.ErrPreconditionFailed, "revision reference receipt identity differs")
	}
	return record, nil
}

// Read views may shorten a caller's context, never extend their own lifetime.
func (c *service) referenceContext(ctx context.Context) (context.Context, func()) {
	if ctx == nil {
		ctx = context.Background()
	}
	if c.readContext == nil {
		return ctx, func() {}
	}
	combined, cancel := context.WithCancel(ctx)
	stop := context.AfterFunc(c.readContext, cancel)
	done := func() {}
	if deadline, ok := c.readContext.Deadline(); ok {
		combined, done = context.WithDeadline(combined, deadline)
	}
	if c.readContext.Err() != nil {
		cancel()
	}
	return combined, func() { stop(); done(); cancel() }
}

func (c *service) finishReferenceRead(record *RevisionReference, err *error) {
	c.finishSourceRead(err)
	if *err != nil {
		*record = RevisionReference{}
	}
}
