package dataset

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"io"

	"kc/kernel"
	"kc/snapshot"
)

func (g *registry) publicationAdmission(head string) (PublicationAdmission, error) {
	authority, err := g.publicationAuthority(head)
	if err != nil {
		return PublicationAdmission{}, err
	}
	var id [16]byte
	if _, err := rand.Read(id[:]); err != nil {
		return PublicationAdmission{}, err
	}
	return PublicationAdmission{PublicationAuthority: authority, ID: hex.EncodeToString(id[:])}, nil
}

func validAdmission(id string) bool {
	decoded, err := hex.DecodeString(id)
	return len(id) == 32 && err == nil && hex.EncodeToString(decoded) == id
}

func publicationReceiptPath(lookup PublicationLookup) string {
	return "_publication/receipts/" + string(kernel.CanonicalDigest(lookup)) + ".json"
}

func validatePublicationReceipt(receipt PublicationReceipt) error {
	if receipt.Version != 1 || !validAdmission(receipt.Admission) || receipt.SetID == "" || receipt.Revision <= 0 || receipt.Definition == "" || receipt.Authority.AuthorityID == "" || receipt.Authority.Binding == "" || receipt.Authority.Head == "" || receipt.Authority.Ref == "" {
		return kernel.Fail(kernel.ErrPreconditionFailed, "publication receipt is incomplete or unsupported")
	}
	return nil
}

func (g *registry) publicationReceiptChange(receipt PublicationReceipt, state State, base kernel.CommitID) (snapshot.TreeChange, error) {
	if err := validatePublicationReceipt(receipt); err != nil {
		return snapshot.TreeChange{}, err
	}
	authority, err := g.publicationAuthority(string(base))
	if err != nil {
		return snapshot.TreeChange{}, err
	}
	if authority != receipt.Authority {
		return snapshot.TreeChange{}, kernel.Fail(kernel.ErrPreconditionFailed, "publication receipt belongs to different authority coordinates")
	}
	found := false
	for _, def := range state.DatasetVersions {
		if def.SetID == receipt.SetID && def.Revision == receipt.Revision && kernel.CanonicalDigest(def) == receipt.Definition {
			found = true
			break
		}
	}
	if !found {
		return snapshot.TreeChange{}, kernel.Fail(kernel.ErrPreconditionFailed, "publication receipt does not match the accepted candidate")
	}
	path := publicationReceiptPath(receipt.PublicationLookup)

	body, err := json.Marshal(receipt)
	return snapshot.TreeChange{Path: path, Content: body}, err
}

// Receipt absence is proved at the admitted immutable base before taking any
// commit lock. The final authority CAS excludes every intervening writer.
func (g *registry) preparePublicationReceipt(ctx context.Context, receipt PublicationReceipt) error {
	registry, err := g.boundIO(ctx)
	if err != nil {
		return err
	}
	path := publicationReceiptPath(receipt.PublicationLookup)
	if _, err := registry.tree.ReadFile(path, receipt.Authority.Head); kernel.CodeOf(err) != kernel.ErrObjectRefUnresolved {
		if err != nil {
			return err
		}
		return kernel.Fail(kernel.ErrPreconditionFailed, "immutable publication receipt already exists")
	}
	return nil
}

func (g *registry) publicationReceiptAt(head kernel.CommitID, lookup PublicationLookup) (PublicationReceipt, error) {
	if lookup.SetID == "" || lookup.Revision <= 0 {
		return PublicationReceipt{}, kernel.Fail(kernel.ErrUsageInvalid, "publication receipt requires a Dataset and positive revision")
	}
	raw, err := g.tree.ReadFile(publicationReceiptPath(lookup), head)
	if err != nil {
		return PublicationReceipt{}, err
	}
	var receipt PublicationReceipt
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&receipt); err != nil {
		return PublicationReceipt{}, kernel.Fail(kernel.ErrPreconditionFailed, "publication receipt is corrupt: %v", err)
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return PublicationReceipt{}, kernel.Fail(kernel.ErrPreconditionFailed, "publication receipt has trailing content")
	}
	if err := validatePublicationReceipt(receipt); err != nil {
		return PublicationReceipt{}, err
	}
	authority, err := g.publicationAuthority(string(head))
	if err != nil {
		return PublicationReceipt{}, err
	}
	if receipt.PublicationLookup != lookup || receipt.Authority.AuthorityID != authority.AuthorityID || receipt.Authority.Binding != authority.Binding || receipt.Authority.Ref != authority.Ref {
		return PublicationReceipt{}, kernel.Fail(kernel.ErrPreconditionFailed, "publication receipt has different identity or authority binding")
	}
	return receipt, nil
}

func (g *registry) publicationReceiptsAt(head kernel.CommitID, lookups []PublicationLookup) ([]PublicationReceipt, error) {
	var receipts []PublicationReceipt
	seen := map[PublicationLookup]bool{}
	for _, lookup := range lookups {
		if seen[lookup] {
			continue
		}
		seen[lookup] = true
		receipt, err := g.publicationReceiptAt(head, lookup)
		if kernel.CodeOf(err) == kernel.ErrObjectRefUnresolved {
			continue
		}
		if err != nil {
			return nil, err
		}
		receipts = append(receipts, receipt)
	}
	return receipts, nil
}

// PublicationReceipt reads the immutable receipt from the actual configured
// authority at one fixed head. It performs no publication or recovery fence.
func (c *service) PublicationReceipt(ctx context.Context, lookup PublicationLookup) (PublicationReceipt, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	registry, err := c.registry.boundIO(ctx)
	if err != nil {
		return PublicationReceipt{}, err
	}
	head, err := registry.authority.Head(registry.ref)
	if err == nil && head == "" {
		err = kernel.Fail(kernel.ErrVersionUnresolved, "publication authority ref is unavailable")
	}
	if err != nil {
		return PublicationReceipt{}, err
	}
	receipt, err := registry.publicationReceiptAt(head, lookup)
	if kernel.CodeOf(err) == kernel.ErrObjectRefUnresolved {
		// Only an existing immutable release on a valid authority can be legacy.
		// Missing authority state or commit is not evidence of a receipt-free release.
		state, stateErr := registry.loadAt(string(head))
		if stateErr != nil {
			return PublicationReceipt{}, stateErr
		}
		if state.AuthorityID != registry.AuthorityID() {
			return PublicationReceipt{}, kernel.Fail(kernel.ErrPreconditionFailed, "publication receipt authority state is invalid")
		}
		for _, definitions := range [][]Dataset{state.DatasetVersions, state.Datasets} {
			for _, definition := range definitions {
				if definition.SetID == lookup.SetID && definition.Revision == lookup.Revision {
					return PublicationReceipt{}, nil
				}
			}
		}
		return PublicationReceipt{}, kernel.Fail(kernel.ErrVersionUnresolved, "publication release is unavailable")
	}
	return receipt, err
}
