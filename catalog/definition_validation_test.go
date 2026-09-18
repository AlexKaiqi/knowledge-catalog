package catalog_test

import (
	"kc/catalog"
	"kc/kernel"
	"kc/snapshot"
	"testing"
)

func TestValidateKnowledgeSetNeedsNoAuthorityOrLiveSelector(t *testing.T) {
	valid := catalog.KnowledgeSet{Revision: 1, Sources: []catalog.KnowledgeSetSource{{Repository: "kr://acme/docs", Selector: snapshot.DefaultRef, BaseRev: "old-fixed"}}}
	if err := catalog.ValidateKnowledgeSet(valid); err != nil {
		t.Fatal(err)
	}
	for name, def := range map[string]catalog.KnowledgeSet{
		"missing repository": {Sources: []catalog.KnowledgeSetSource{{Selector: snapshot.DefaultRef}}},
		"missing selector":   {Sources: []catalog.KnowledgeSetSource{{Repository: "kr://acme/source"}}},
		"empty":              {},
		"retired":            {Retired: true, Sources: valid.Sources},
		"duplicate":          {Sources: append(valid.Sources, valid.Sources...)},
		"overlapping mounts": {Sources: []catalog.KnowledgeSetSource{{Repository: "kr://acme/docs", Selector: snapshot.DefaultRef, Path: catalog.MountPath("docs"), SubPath: "a"}, {Repository: "kr://acme/docs", Selector: snapshot.DefaultRef, Path: catalog.MountPath("more"), SubPath: "a/b"}}},
	} {
		t.Run(name, func(t *testing.T) {
			if err := catalog.ValidateKnowledgeSet(def); kernel.CodeOf(err) != kernel.ErrKnowledgeSetInvalid {
				t.Fatalf("invalid shape: %v", err)
			}
		})
	}
}
