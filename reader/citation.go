package reader

import (
	"kc/kernel"
	"kc/repository"
)

// GroundingCitation is a consume-side projection of a READ result (D12 / B.1).
// Not a write surface, not GET_PROVENANCE, not a Catalog object.

type GroundingCitation struct {
	KnowledgeRef      kernel.KnowledgeRef `json:"knowledgeRef"`
	PinnedRef         string              `json:"pinnedRef"`
	Digest            kernel.Digest       `json:"digest,omitempty"`
	Fragment          string              `json:"fragment,omitempty"`
	ProvenanceSummary *ProvenanceSummary  `json:"provenanceSummary,omitempty"`
}

type ProvenanceSummary struct {
	ActorRef   string   `json:"actorRef,omitempty"`
	SourceRefs []string `json:"sourceRefs,omitempty"`
	OriginKind string   `json:"originKind,omitempty"`
}

func NewGroundingCitation(value repository.KnowledgeValue) GroundingCitation {
	citation := GroundingCitation{
		KnowledgeRef: value.KnowledgeRef,
		PinnedRef:    kernel.FormatPinnedRef(value.Repository, value.Commit, value.Address.ObjectID),
		Digest:       kernel.CanonicalDigest(value.Value),
	}
	if value.Provenance != nil {
		citation.ProvenanceSummary = &ProvenanceSummary{
			ActorRef:   value.Provenance.ActorRef,
			SourceRefs: value.Provenance.SourceRefs,
			OriginKind: string(value.Provenance.OriginKind),
		}
	}
	return citation
}
