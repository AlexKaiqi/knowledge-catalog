package repository_test

import (
	"testing"

	"kc/internal/testkit"
	"kc/kernel"
	"kc/repository"
	"kc/scale"
)

func TestT12FileGitContract(t *testing.T) {
	factory := func(t *testing.T, id string) repository.Repository {
		return testkit.MakeRepository(t, id)
	}
	testkit.RepositoryContract(t, factory)
	testkit.WriterContract(t, factory)
}

func TestT12DoltContract(t *testing.T) {
	factory := func(t *testing.T, id string) repository.Repository {
		t.Helper()
		repo, err := scale.OpenDolt(testkit.TempDir(t), kernel.RepositoryID(id))
		if err != nil {
			t.Fatal(err)
		}
		return repo
	}
	testkit.RepositoryContract(t, factory)
	testkit.WriterContract(t, factory)
}
