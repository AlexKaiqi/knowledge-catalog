package knowledge_test

import (
	"testing"

	"kc/internal/testkit"
	"kc/snapshot"
)

func TestT12FileGitContract(t *testing.T) {
	factory := func(t *testing.T, id string) snapshot.Store {
		return testkit.MakeRepository(t, id)
	}
	testkit.RepositoryContract(t, factory)
	testkit.WriterContract(t, factory)
}
