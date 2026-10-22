package object

import (
	"encoding/json"
	"testing"

	"kc/kernel"
)

func testReadBasis() ReadBasis {
	return ReadBasis{Version: 1, Reference: "urn:kc:object:table/orders", Dataset: "orders", Revision: 1,
		Candidates:   []ReadSource{{Repository: "kr://trade", Commit: "a1", Binding: "store-a"}, {Repository: "kr://ops", Commit: "b1", Binding: "store-b"}},
		Dependencies: []ReadSource{{Repository: "kr://descriptors", Commit: "d1", Binding: "store-d"}},
		Paths:        []ReadPath{{Repository: "kr://trade", Kind: "prefix"}, {Repository: "kr://ops", Kind: "prefix", Prefix: "public"}}}
}

func TestReadBasisScopeIsIndependentOfVersionAndUnitMembership(t *testing.T) {
	basis := testReadBasis()
	want, err := basis.ScopeDigest()
	if err != nil {
		t.Fatal(err)
	}
	for name, mutate := range map[string]func(*ReadBasis){
		"source commit":      func(b *ReadBasis) { b.Candidates[0].Commit = "a2" },
		"revision":           func(b *ReadBasis) { b.Revision++ },
		"dependency version": func(b *ReadBasis) { b.Dependencies[0].Commit = "d2" },
		"candidate order":    func(b *ReadBasis) { b.Candidates[0], b.Candidates[1] = b.Candidates[1], b.Candidates[0] },
		"path order":         func(b *ReadBasis) { b.Paths[0], b.Paths[1] = b.Paths[1], b.Paths[0] },
	} {
		t.Run(name, func(t *testing.T) {
			b := testReadBasis()
			mutate(&b)
			got, err := b.ScopeDigest()
			if err != nil || got != want {
				t.Fatalf("version/order changed scope: %s %v", got, err)
			}
		})
	}
	for name, mutate := range map[string]func(*ReadBasis){
		"candidate": func(b *ReadBasis) { b.Candidates = b.Candidates[:1]; b.Paths = b.Paths[:1] },
		"binding":   func(b *ReadBasis) { b.Candidates[0].Binding = "other-instance" },
		"selection": func(b *ReadBasis) { b.Paths[1].Prefix = "public/narrower" },
		"locator":   func(b *ReadBasis) { b.Reference += "#/definition" },
		"shape":     func(b *ReadBasis) { b.Shape = "{ definition { name } }" },
	} {
		t.Run(name, func(t *testing.T) {
			b := testReadBasis()
			mutate(&b)
			got, err := b.ScopeDigest()
			if err != nil || got == want {
				t.Fatalf("selection change lost: %s %v", got, err)
			}
		})
	}
	raw, err := json.Marshal(basis)
	if err != nil {
		t.Fatal(err)
	}
	var reopened ReadBasis
	if err := json.Unmarshal(raw, &reopened); err != nil {
		t.Fatal(err)
	}
	if got, err := reopened.ScopeDigest(); err != nil || got != want {
		t.Fatalf("serialized basis changed: %s %v", got, err)
	}
}

func TestReadBasisRejectsIncompleteOrAmbiguousCoordinates(t *testing.T) {
	for name, mutate := range map[string]func(*ReadBasis){
		"encoding version":    func(b *ReadBasis) { b.Version++ },
		"no candidates":       func(b *ReadBasis) { b.Candidates = nil },
		"unfixed source":      func(b *ReadBasis) { b.Candidates[0].Commit = "" },
		"unbound source":      func(b *ReadBasis) { b.Candidates[0].Binding = "" },
		"duplicate candidate": func(b *ReadBasis) { b.Candidates = append(b.Candidates, b.Candidates[0]) },
		"unfixed dependency":  func(b *ReadBasis) { b.Dependencies[0].Commit = "" },
		"inconsistent binding": func(b *ReadBasis) {
			b.Dependencies = append(b.Dependencies, ReadSource{Repository: b.Candidates[0].Repository, Commit: "a2", Binding: "changed"})
		},
		"path outside candidates":  func(b *ReadBasis) { b.Paths[0].Repository = "kr://outside" },
		"invalid selector":         func(b *ReadBasis) { b.Paths[0].Kind = "unknown" },
		"ambiguous selector":       func(b *ReadBasis) { b.Paths[0].File = "secret.yaml" },
		"empty file":               func(b *ReadBasis) { b.Paths[0].Kind = "file" },
		"relative traversal":       func(b *ReadBasis) { b.Paths[1].Prefix = "public/../private" },
		"dataset without revision": func(b *ReadBasis) { b.Revision = 0 },
		"revision without dataset": func(b *ReadBasis) { b.Dataset = "" },
		"locator with shape":       func(b *ReadBasis) { b.Reference += "#/definition"; b.Shape = "{ definition }" },
	} {
		t.Run(name, func(t *testing.T) {
			b := testReadBasis()
			mutate(&b)
			if err := b.Validate(); kernel.CodeOf(err) != kernel.ErrUsageInvalid {
				t.Fatalf("invalid basis accepted: %v", err)
			}
		})
	}
}

func TestReadBasisScopeNormalizesEquivalentSelectionSpelling(t *testing.T) {
	basis := testReadBasis()
	basis.Shape = `{ definition { name owner } columns(keys: ["id", "status"]) { type } }`
	want, err := basis.ScopeDigest()
	if err != nil {
		t.Fatal(err)
	}
	basis.Reference = "URN:KC:object:table/orders"
	basis.Shape = `{ columns(keys: ["status", "id"]) { type } definition { owner name } }`
	basis.Paths = append(basis.Paths, basis.Paths[0])
	if got, err := basis.ScopeDigest(); err != nil || got != want {
		t.Fatalf("equivalent selection changed logical scope: %s %v", got, err)
	}
}
