package arch_test

import (
	"fmt"
	"testing"
)

func TestLiveAccessFixtureAdoptionIsExplicitTestInput(t *testing.T) {
	for _, signal := range liveAccessFixtureImportSignals(loadGraph(t)) {
		t.Error(signal)
	}
	if got := liveAccessFixtureImportSignals(depGraph{"cli": {"liveaccess/fixture"}}); len(got) != 1 {
		t.Fatalf("production auto-adoption was accepted: %v", got)
	}
}

func liveAccessFixtureImportSignals(graph depGraph) []string {
	var signals []string
	for owner, dependencies := range graph {
		for _, dependency := range dependencies {
			if deniedMatch("liveaccess/fixture", dependency) {
				signals = append(signals, fmt.Sprintf("production %s imports test adoption helper %s", owner, dependency))
			}
		}
	}
	return signals
}
