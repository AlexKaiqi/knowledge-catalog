package home

import (
	"kc/internal/testkit"
	objectmodel "kc/object"
	"testing"

	"kc/snapshot"
)

func TestManagedRestoreKeepsDriverCapabilities(t *testing.T) {
	original := authorityDrivers["lakefs"]
	native := testkit.MakeNativeSystemRepository(t)
	head, err := native.Head(snapshot.DefaultRef)
	if err != nil {
		t.Fatal(err)
	}
	opened := 0
	driver := original
	driver.managedOpen = func(RepositoryBinding, string, string) (snapshot.Store, error) { opened++; return native, nil }
	authorityDrivers["lakefs"] = driver
	defer func() { authorityDrivers["lakefs"] = original }()
	restored, err := restoreManagedSource(managedRecord{Binding: RepositoryBinding{Driver: "lakefs"}, Head: head})
	if err != nil {
		t.Fatal(err)
	}
	if restored != native || opened != 1 {
		t.Fatal("lakeFS restoration replaced its driver handle")
	}
	if _, ok := restored.(objectmodel.NativeRepository); !ok {
		t.Fatal("native object capability was erased")
	}
	if _, ok := restored.(snapshot.TreeStore); ok {
		t.Fatal("restoration invented a TreeStore capability")
	}
}
