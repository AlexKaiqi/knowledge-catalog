package datasets

import (
	"context"
	"encoding/json"

	"kc/snapshot/dataset"
)

// PublicationProtection is one managed build's durable preparation handle. It
// protects the exact physical generation a build produced, so a crash between
// "corpus written" and "release confirmed ready" can neither lose that
// generation nor let another writer silently adopt it by name.
//
// It is no longer a publication precondition: acceptance is decided before any
// corpus exists (doc/design/dataset.md#发布), so the ticket now belongs to the
// build that follows acceptance, not to the request that produced the version.
type PublicationProtection interface {
	Admit(context.Context, []byte) error
	Confirm(context.Context) error
	Cancel(context.Context) error
	Close() error
}

// The full accepted candidate is retained with the authority coordinates, so
// recovery can match its exact intent against immutable authority state. This
// evidence belongs to a preparation claim, not to a source membership list.
type publicationResourceEvidence struct {
	Version    int                          `json:"version"`
	Admission  string                       `json:"admission,omitempty"`
	Authority  dataset.PublicationAuthority `json:"authority"`
	Definition dataset.Dataset              `json:"definition"`
}

func marshalPublicationEvidence(def dataset.Dataset, authority dataset.PublicationAdmission) ([]byte, error) {
	return json.Marshal(publicationResourceEvidence{Version: 2, Authority: authority.PublicationAuthority, Admission: authority.ID, Definition: def})
}
