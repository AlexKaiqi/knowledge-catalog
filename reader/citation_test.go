package reader_test

import (
	"testing"

	"kc/kernel"
	"kc/reader"
	"kc/repository"
)

func TestT7GroundingCitation(t *testing.T) {
	value := repository.KnowledgeValue{
		KnowledgeRef: kernel.KnowledgeRef{Repository: "kr://acme/public/core", Object: "policy/P-103"},
		Repository:   "kr://acme/public/core",
		Commit:       "abc123",
		Address:      kernel.Address{Kind: kernel.KindEntity, ObjectID: "policy/P-103"},
		Value:        map[string]any{"statement": "v1"},
		Provenance:   &kernel.ProvenanceEnvelope{OriginKind: kernel.OriginDefinition, ActorRef: "core-council", SourceRefs: []string{"handbook-v1"}},
	}
	citation := reader.NewGroundingCitation(value)
	if citation.PinnedRef != "kc://acme/public/core@abc123/policy/P-103" {
		t.Fatal(citation.PinnedRef)
	}
	if citation.Digest != kernel.CanonicalDigest(value.Value) {
		t.Fatal(citation.Digest)
	}
	if citation.ProvenanceSummary == nil || citation.ProvenanceSummary.OriginKind != "DEFINITION" || citation.ProvenanceSummary.ActorRef != "core-council" {
		t.Fatalf("%#v", citation.ProvenanceSummary)
	}
}
