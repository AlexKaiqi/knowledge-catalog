package cli_test

import (
	"testing"

	kchome "kc/home"
	"kc/kernel"
	objectmodel "kc/object"
	"kc/snapshot"
)

func TestSceneExistingRepositoryHasCompleteEmptyObjectMetadata(t *testing.T) {
	cache := newSceneHomeCache(t)
	cache.ensureFrozen(t, cache.nodes["source-repositories-configured"], nil)
	clone := cache.cloneWorld(t, "source-repositories-configured")
	defer clone.close()
	for _, dir := range []string{cache.frozen["source-repositories-configured"].dir, clone.home} {
		ws, err := kchome.Open(dir)
		if err != nil {
			t.Fatal(err)
		}
		for _, id := range []kernel.RepositoryID{"kr://scene/object", "kr://scene/graph"} {
			repo, err := ws.Reader.Require(id, kernel.ErrCapabilityUnsatisfied)
			if err != nil {
				t.Fatal(err)
			}
			at, err := repo.Head(snapshot.DefaultRef)
			if err != nil {
				t.Fatal(err)
			}
			addresses, err := repo.(objectmodel.SchemaReferrerLocator).SchemaReferrerAddresses(objectmodel.IndexDeclarationSchemaV1, at)
			if err != nil || len(addresses) != 0 {
				t.Fatalf("empty object source lacks complete declaration metadata after freeze/fork: repository=%s commit=%s referrers=%v error=%v", id, at, addresses, err)
			}
		}
		_ = ws.Close()
	}
}
