package cli_test

import (
	"fmt"
	"strings"
	"testing"
)

func TestDatasetScenePublishesItsSchemaDependencies(t *testing.T) {
	cache := newSceneHomeCache(t)
	node := cache.nodes["knowledge-set-defined"]
	world := cache.replayConstructs(t, append(append([]string{}, node.Ancestors...), node.ID), &sceneRunReport{})
	defer world.close()
	plan := body(t, kc(world.home, "operations", "access-spec", "describe", "--dataset", "scene-set"))
	for _, schema := range []string{"schema/metric.definition", "schema/core/relation/v1"} {
		if !strings.Contains(fmt.Sprint(plan), schema) {
			t.Fatalf("Dataset did not publish dependency %s: %#v", schema, plan)
		}
	}
}
