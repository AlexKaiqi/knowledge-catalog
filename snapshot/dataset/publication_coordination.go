package dataset

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"sync"
	"time"

	"kc/kernel"
	"kc/snapshot"
)

// PublicationAuthority is the exact registry ref version against which an
// external preparation may admit publication. Binding separates deployments
// and branch refs that happen to have the same logical authority name.
type PublicationAuthority struct {
	AuthorityID kernel.RepositoryID `json:"authorityId"`
	Binding     kernel.Digest       `json:"binding"`
	Ref         string              `json:"ref"`
	Head        kernel.CommitID     `json:"head"`
}

// PublicationAdmission identifies one preparation attempt at an exact authority
// version. Equal Dataset candidates still receive distinct admission IDs.
type PublicationAdmission struct {
	PublicationAuthority
	ID string `json:"id"`
}

// PublicationLookup names one immutable Dataset release, not a resource.
type PublicationLookup struct {
	SetID    string `json:"setId"`
	Revision int    `json:"revision"`
}

// PublicationReceipt is committed atomically with the accepted Dataset. Its
// opaque admission identity links external preparation without interpreting it.
type PublicationReceipt struct {
	Version int `json:"version"`
	PublicationLookup
	Admission  string               `json:"admission"`
	Definition kernel.Digest        `json:"definition"`
	Authority  PublicationAuthority `json:"authority"`
}

// PublicationCheckpoint is an accepted ref fence plus the immutable registry
// state at that fence. It creates neither a Dataset revision nor file delivery.
type PublicationCheckpoint struct {
	Authority PublicationAuthority `json:"authority"`
	State     State                `json:"state"`
	Receipts  []PublicationReceipt `json:"receipts,omitempty"`
}

// PublicationCoordinator is an optional file-authority port. It knows no
// indexes or resources: the admission callback persists their own protection.
// Recovery must match the authority binding before using a checkpoint.
type PublicationCoordinator interface {
	PublishDatasetGuarded(context.Context, Dataset, func(context.Context, PublicationAdmission) error) (Dataset, error)
	FencePublications(context.Context, ...PublicationLookup) (PublicationCheckpoint, error)
	PublicationReceipt(context.Context, PublicationLookup) (PublicationReceipt, error)
}

const publicationFenceFile = "_publication/fence.json"

func (g *registry) publicationBinding() (kernel.Digest, error) {
	origin, ok := g.authority.(snapshot.StoreOrigin)
	originKey := ""
	if ok {
		originKey = origin.Origin()
	}
	if originKey == "" {
		return "", kernel.Fail(kernel.ErrCapabilityUnsatisfied, "publication coordination requires a stable registry authority binding")
	}
	return kernel.CanonicalDigest(struct{ ID, Origin, Ref string }{g.AuthorityID(), originKey, g.ref}), nil
}

func (g *registry) publicationAuthority(head string) (PublicationAuthority, error) {
	binding, err := g.publicationBinding()
	if err != nil {
		return PublicationAuthority{}, err
	}
	if head == "" {
		return PublicationAuthority{}, kernel.Fail(kernel.ErrVersionUnresolved, "publication coordination requires a fixed registry head")
	}
	return PublicationAuthority{AuthorityID: g.authority.ID(), Binding: binding, Ref: g.ref, Head: kernel.CommitID(head)}, nil
}

func (c *service) PublishDatasetGuarded(ctx context.Context, def Dataset, admit func(context.Context, PublicationAdmission) error) (Dataset, error) {
	if admit == nil {
		return Dataset{}, kernel.Fail(kernel.ErrUsageInvalid, "protected publication requires an admission callback")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	return c.publishDataset(ctx, def, admit)
}

// FencePublications reads the actual configured authority, rather than cached
// accepted state, and advances its ref by CAS. An older guarded publisher can
// no longer apply after this succeeds, even if it resumes in another process.
// Unknown commit outcomes return no checkpoint; the caller must retain safety
// protection and retry recovery against the actual authority.
func (c *service) FencePublications(ctx context.Context, lookups ...PublicationLookup) (checkpoint PublicationCheckpoint, err error) {
	if err := c.ensureWritable(); err != nil {
		return checkpoint, err
	}
	if ctx == nil {
		ctx = context.Background()
	}
	defer func() {
		err = c.note("dataset-publication-fence", nil, err)
		if err != nil {
			checkpoint = PublicationCheckpoint{}
		}
	}()
	unlock, err := lockForContext(ctx, &c.writeMu)
	if err != nil {
		return checkpoint, err
	}
	defer unlock()
	unlockRegistry, err := lockForContext(ctx, &c.registry.writeMu)
	if err != nil {
		return checkpoint, err
	}
	defer unlockRegistry()
	io, err := c.registry.boundIO(ctx)
	if err != nil {
		return checkpoint, err
	}
	head, err := io.authority.Head(io.ref)
	if err != nil {
		return checkpoint, err
	}
	version, err := c.registry.publicationAuthority(string(head))
	if err != nil {
		return checkpoint, err
	}
	state, err := io.loadAt(string(head))
	if err != nil {
		return checkpoint, err
	}
	if state.AuthorityID != string(version.AuthorityID) {
		return checkpoint, kernel.Fail(kernel.ErrPreconditionFailed, "publication fence cannot initialize or replace a registry authority")
	}
	receipts, err := io.publicationReceiptsAt(head, lookups)
	if err != nil {
		return checkpoint, err
	}
	var nonce [16]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return checkpoint, err
	}
	body, err := json.Marshal(struct {
		Version int             `json:"version"`
		Nonce   string          `json:"nonce"`
		Base    kernel.CommitID `json:"base"`
	}{1, hex.EncodeToString(nonce[:]), head})
	if err != nil {
		return checkpoint, err
	}
	if err := ctx.Err(); err != nil {
		return checkpoint, err
	}
	as, requestID, ruleID, _ := c.requestContext()
	next, err := io.tree.ApplyTreeCommit(snapshot.TreeChangeSet{
		TargetRepository: version.AuthorityID, TargetRef: version.Ref,
		BaseCommit: head, ExpectedTargetCommit: head,
		Changes: []snapshot.TreeChange{{Path: publicationFenceFile, Content: body}},
		Message: "dataset-publication-fence", Author: as, RequestID: requestID, RuleID: ruleID,
	})
	if err != nil {
		return checkpoint, err
	}
	if next == "" || next == head {
		return checkpoint, kernel.Fail(kernel.ErrTemporaryUnavailable, "publication fence did not confirm a new authority version")
	}
	c.registry.accept(string(next))
	c.current.Store(newAcceptedState(state, string(next)))
	version.Head = next
	return PublicationCheckpoint{Authority: version, State: c.accepted().dump(c.registry.AuthorityID()), Receipts: receipts}, nil
}

func lockForContext(ctx context.Context, mutex *sync.Mutex) (func(), error) {
	if ctx == nil || ctx.Done() == nil {
		mutex.Lock()
		return mutex.Unlock, nil
	}
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if mutex.TryLock() {
			return mutex.Unlock, nil
		}
		timer := time.NewTimer(5 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, ctx.Err()
		case <-timer.C:
		}
	}
}

// Both new publication and preparation adoption must accept the exact original
// authority coordinates. Neither path may rebase after external admission.
func (c *service) checkAdmittedAuthority(head string, admitted PublicationAuthority) error {
	if head != string(admitted.Head) {
		return kernel.Fail(kernel.ErrNonFastForward, "publication admission belongs to an older registry ref version")
	}
	actual, err := c.registry.publicationAuthority(head)
	if err != nil {
		return err
	}
	if actual != admitted {
		return kernel.Fail(kernel.ErrPreconditionFailed, "publication authority changed after admission")
	}
	return nil
}
