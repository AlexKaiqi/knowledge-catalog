package opensearch

import (
	"testing"

	"kc/kernel"
)

func TestPITContinuationRejectsTampering(t *testing.T) {
	token := encodePITContinuation(pitContinuation{
		PIT: "pit-1", Basis: kernel.CommitID("commit-1"), Repository: kernel.RepositoryID("kr://acme/public/core"),
		Query: "query-1", Generation: "generation-1", Sort: []any{"relation:one"}, Rank: 1,
	})
	position := len(token) / 2
	replacement := "A"
	if token[position] == 'A' {
		replacement = "B"
	}
	tampered := token[:position] + replacement + token[position+1:]
	if _, err := decodePITContinuation(tampered); err == nil {
		t.Fatal("tampered continuation must be rejected")
	}
	decoded, err := decodePITContinuation(token)
	if err != nil {
		t.Fatal(err)
	}
	if decoded.Basis != "commit-1" || decoded.Query != "query-1" || decoded.Generation != "generation-1" {
		t.Fatalf("continuation binding was lost: %#v", decoded)
	}
}
