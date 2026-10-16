package cli_test

import (
	"fmt"
	"strings"
	"testing"
)

func TestDatasetScenePublishesItsSchemaDependencies(t *testing.T) {
	cache := newSceneHomeCache(t)
	node := cache.nodes["semantic-dataset-defined"]
	world := cache.replayConstructs(t, append(append([]string{}, node.Ancestors...), node.ID), &sceneRunReport{})
	defer world.close()
	// The Dataset delivers each dependency Schema itself; access fields are
	// no longer the witness, because Schema carries structure only.
	for _, schema := range []string{"schema/metric.definition", "schema/core/relation/v1"} {
		described := body(t, kc(world.home, "schema", "describe", "--dataset", "scene-set", "--object", schema))
		if !strings.Contains(fmt.Sprint(described), schema) {
			t.Fatalf("Dataset did not publish dependency %s: %#v", schema, described)
		}
	}
	if absent := fmt.Sprint(body(t, kc(world.home, "schema", "describe", "--dataset", "scene-set", "--object", "schema/scene-undelivered"))); strings.Contains(absent, "schema/scene-undelivered") {
		t.Fatalf("describe reported a Schema the Dataset does not deliver: %s", absent)
	}
}
