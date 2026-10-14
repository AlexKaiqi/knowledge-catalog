package home

import (
	"testing"

	"kc/kernel"
	objectmodel "kc/object"
)

func TestRepositoryAccessRejectsWriteActions(t *testing.T) {
	err := ValidateAuthenticatedActions([]string{"object.read", "writer.commit"})
	if kernel.CodeOf(err) != kernel.ErrUsageInvalid {
		t.Fatalf("write action must not be an authenticated default: %v", err)
	}
}

func TestEnsureRepositoryAccessDoesNotRestoreRemovedEntry(t *testing.T) {
	dir := t.TempDir()
	if err := EnsureRepositoryAccess(dir, SystemRepositoryAccess()); err != nil {
		t.Fatal(err)
	}
	file, err := ReadRepositoryAccess(dir)
	if err != nil || !file.Allows(string(objectmodel.SystemRepositoryID), "object.read") {
		t.Fatalf("first ensure should seed the protocol publication: %#v %v", file, err)
	}
	if err := WriteRepositoryAccess(dir, RepositoryAccessFile{}); err != nil {
		t.Fatal(err)
	}
	if err := EnsureRepositoryAccess(dir, SystemRepositoryAccess()); err != nil {
		t.Fatal(err)
	}
	file, err = ReadRepositoryAccess(dir)
	if err != nil || file.Allows(string(objectmodel.SystemRepositoryID), "object.read") {
		t.Fatalf("second ensure must not restore a removed default: %#v %v", file, err)
	}
}
