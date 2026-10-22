package dataset

import (
	"context"

	"kc/kernel"
)

// PublicationInspector reads actual authority state and selected immutable
// receipts at one head, without excluding pending publishers. It is suitable
// for accepted-reference lifecycle checks, never for discharging an unresolved
// admission whose original CAS may still succeed.
type PublicationInspector interface {
	InspectPublications(context.Context, ...PublicationLookup) (PublicationCheckpoint, error)
}

func (c *service) InspectPublications(ctx context.Context, lookups ...PublicationLookup) (result PublicationCheckpoint, err error) {
	ctx, done := c.referenceContext(ctx)
	defer done()
	defer func() {
		if contextErr := ctx.Err(); contextErr != nil {
			err = contextErr
		}
		c.finishSourceRead(&err)
		if err != nil {
			result = PublicationCheckpoint{}
		}
	}()
	io, head, err := c.referenceIO(ctx)
	if err != nil {
		return PublicationCheckpoint{}, err
	}
	authority, err := io.publicationAuthority(string(head))
	if err != nil {
		return PublicationCheckpoint{}, err
	}
	state, err := io.loadAt(string(head))
	if err != nil {
		return PublicationCheckpoint{}, err
	}
	if state.AuthorityID != string(authority.AuthorityID) {
		return PublicationCheckpoint{}, kernel.Fail(kernel.ErrPreconditionFailed, "publication inspection authority state is invalid")
	}
	receipts, err := io.publicationReceiptsAt(head, lookups)
	if err != nil {
		return PublicationCheckpoint{}, err
	}
	return PublicationCheckpoint{Authority: authority, State: newAcceptedState(state, string(head)).dump(io.AuthorityID()), Receipts: receipts}, nil
}
