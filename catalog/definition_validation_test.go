package catalog_test

import (
	"kc/catalog"
	"kc/kernel"
	"kc/snapshot"
	"testing"
)

func TestValidateWorkspaceDefinitionNeedsNoAuthorityOrLiveSelector(t *testing.T) {
	valid := catalog.WorkspaceDefinition{Revision: 1, Sources: []catalog.WorkspaceSource{{Repository: "kr://acme/docs", Selector: snapshot.DefaultRef, BaseRev: "old-fixed"}}}
	if err := catalog.ValidateWorkspaceDefinition(valid); err != nil {
		t.Fatal(err)
	}
	for name, def := range map[string]catalog.WorkspaceDefinition{
		"missing repository": {Sources: []catalog.WorkspaceSource{{Selector: snapshot.DefaultRef}}},
		"missing selector":   {Sources: []catalog.WorkspaceSource{{Repository: "kr://acme/source"}}},
		"empty":              {},
		"retired":            {Retired: true, Sources: valid.Sources},
		"duplicate":          {Sources: append(valid.Sources, valid.Sources...)},
		"overlapping mounts": {Sources: []catalog.WorkspaceSource{{Repository: "kr://acme/docs", Selector: snapshot.DefaultRef, Path: catalog.MountPath("docs"), SubPath: "a"}, {Repository: "kr://acme/docs", Selector: snapshot.DefaultRef, Path: catalog.MountPath("more"), SubPath: "a/b"}}},
	} {
		t.Run(name, func(t *testing.T) {
			if err := catalog.ValidateWorkspaceDefinition(def); kernel.CodeOf(err) != kernel.ErrWorkspaceInvalid {
				t.Fatalf("invalid shape: %v", err)
			}
		})
	}
}
