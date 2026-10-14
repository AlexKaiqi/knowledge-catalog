package dataset_test

import (
	"kc/kernel"
	"kc/snapshot"
	catalog "kc/snapshot/dataset"
	"testing"
)

func TestValidateDatasetNeedsNoAuthorityOrLiveSelector(t *testing.T) {
	valid := catalog.Dataset{Revision: 1, Sources: []catalog.DatasetSource{{Repository: "kr://acme/docs", Selector: snapshot.DefaultRef, BaseRev: "old-fixed"}}}
	if err := catalog.ValidateDataset(valid); err != nil {
		t.Fatal(err)
	}
	for name, def := range map[string]catalog.Dataset{
		"missing repository": {Sources: []catalog.DatasetSource{{Selector: snapshot.DefaultRef}}},
		"missing selector":   {Sources: []catalog.DatasetSource{{Repository: "kr://acme/source"}}},
		"empty":              {},
		"retired":            {Retired: true, Sources: valid.Sources},
		"duplicate":          {Sources: append(valid.Sources, valid.Sources...)},
		"overlapping mounts": {Sources: []catalog.DatasetSource{{Repository: "kr://acme/docs", Selector: snapshot.DefaultRef, Path: catalog.MountPath("docs"), SubPath: "a"}, {Repository: "kr://acme/docs", Selector: snapshot.DefaultRef, Path: catalog.MountPath("more"), SubPath: "a/b"}}},
	} {
		t.Run(name, func(t *testing.T) {
			if err := catalog.ValidateDataset(def); kernel.CodeOf(err) != kernel.ErrDatasetInvalid {
				t.Fatalf("invalid shape: %v", err)
			}
		})
	}
}
