package main

import (
	"math"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestUnitAtSeparatesSnapshotHTTPAdapterFromItsCore(t *testing.T) {
	for _, path := range []string{"kc/snapshot/service", "kc/snapshot/service/internal/transport"} {
		unit, ok := unitAt("kc", path, 1)
		if !ok || unit != "snapshot/service" {
			t.Errorf("%s: got unit %q, want the independent HTTP adapter unit", path, unit)
		}
	}
	for _, path := range []string{"kc/snapshot", "kc/snapshot/dataset", "kc/snapshot/access", "kc/snapshot/console", "kc/snapshot/service-tools"} {
		unit, ok := unitAt("kc", path, 1)
		if !ok || unit != "snapshot" {
			t.Errorf("%s: got unit %q, want the Snapshot product core unit", path, unit)
		}
	}
}

func TestUnitPolicyRetainsModuleAndDepthBoundaries(t *testing.T) {
	if _, ok := unitAt("kc", "kc-other/snapshot/service", 1); ok {
		t.Fatal("foreign module was classified as a project unit")
	}
	unit, ok := unitAt("kc", "kc/snapshot/service/internal/transport", 3)
	if !ok || unit != "snapshot/service/internal" {
		t.Fatalf("read-only depth reports lost their path granularity: %q", unit)
	}
	if unitPolicySignature(1) == unitPolicySignature(3) {
		t.Fatal("different path policies must not share a baseline signature")
	}
	if got := relativeUnit("snapshot/service/auth_http.go", 1); got != "snapshot/service" {
		t.Fatalf("co-change source grouping disagrees with the import unit policy: %s", got)
	}
}

func TestCollectRetainsEveryDependencyAcrossNamedUnitBoundary(t *testing.T) {
	t.Setenv("GOWORK", "off")
	root := t.TempDir()
	for path, source := range map[string]string{
		"go.mod":                      "module kc\n\ngo 1.23\n",
		"kernel/core.go":              "package kernel\n",
		"identity/identity.go":        "package identity\nimport _ \"kc/kernel\"\n",
		"snapshot/store.go":           "package snapshot\nimport _ \"kc/kernel\"\n",
		"snapshot/dataset/dataset.go": "package dataset\nimport (_ \"kc/snapshot\"; _ \"kc/kernel\")\n",
		"snapshot/console/console.go": "package console\nimport _ \"net/http\"\n",
		"snapshot/service/service.go": "package service\nimport (_ \"kc/identity\"; _ \"kc/kernel\"; _ \"kc/snapshot\"; _ \"kc/snapshot/dataset\"; _ \"kc/snapshot/console\")\n",
		"server/server.go":            "package server\nimport (_ \"kc/snapshot\"; _ \"kc/snapshot/service\")\n",
	} {
		full := filepath.Join(root, path)
		if err := os.MkdirAll(filepath.Dir(full), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(source), 0600); err != nil {
			t.Fatal(err)
		}
	}
	graph, _, _, _, _, err := collect(root, 1)
	if err != nil {
		t.Fatal(err)
	}
	for unit, want := range map[string][]string{
		"server":           {"snapshot", "snapshot/service"},
		"snapshot/service": {"identity", "kernel", "snapshot"},
		"snapshot":         {"kernel"},
		"identity":         {"kernel"},
		"kernel":           {},
	} {
		got := make([]string, 0, len(graph[unit]))
		for dependency := range graph[unit] {
			got = append(got, dependency)
		}
		slices.Sort(got)
		if !slices.Equal(got, want) {
			t.Errorf("%s lost or rewrote an import: got %v want %v", unit, got, want)
		}
	}
	if len(graph) != 5 {
		t.Fatalf("collection omitted or invented a logical unit: %#v", graph)
	}
	if reach("snapshot", graph, map[string]bool{})["identity"] {
		t.Fatal("HTTP adapter's identity edge was attributed to its core")
	}
}

func TestStrictRejectsMissingDifferentOrWrongDepthPolicy(t *testing.T) {
	for _, base := range []baseline{
		{Depth: 1},
		{Depth: 1, UnitPolicy: "old-top-level-policy"},
		{Depth: 2, UnitPolicy: unitPolicySignature(1)},
	} {
		problems := strictCheck(&base, nil, 0)
		if len(problems) != 1 || !strings.Contains(problems[0], "unit policy mismatch") {
			t.Fatalf("incomparable baseline was accepted: %#v %v", base, problems)
		}
	}
}

func TestStrictRetainsUnknownUnitPropagationAndInstabilityGates(t *testing.T) {
	base := baseline{Depth: defaultUnitDepth, UnitPolicy: unitPolicySignature(defaultUnitDepth), PropagationCost: 0.25,
		Units: []unitStat{{Unit: "snapshot", Instability: 0.125}}}
	for _, trial := range []struct {
		stats       []unitStat
		propagation float64
		want        string
	}{
		{[]unitStat{{Unit: "snapshot/service"}}, 0, "unit snapshot/service is not in the baseline"},
		{[]unitStat{{Unit: "unexpected-capability"}}, 0, "unit unexpected-capability is not in the baseline"},
		{base.Units, 0.3, "propagation cost"},
		{[]unitStat{{Unit: "snapshot", Instability: 0.175}}, 0.25, "instability drift"},
	} {
		problems := strictCheck(&base, trial.stats, trial.propagation)
		if !strings.Contains(strings.Join(problems, "\n"), trial.want) {
			t.Fatalf("existing hard gate stopped rejecting its signal: want %q, got %v", trial.want, problems)
		}
	}
	if problems := strictCheck(&base, base.Units, base.PropagationCost); len(problems) != 0 {
		t.Fatalf("unchanged comparable measurements failed: %v", problems)
	}
}

func TestPropagationRetainsEdgesAndReachabilityArithmetic(t *testing.T) {
	graph := map[string]map[string]bool{
		"server": {"snapshot/service": true}, "snapshot/service": {"identity": true, "snapshot": true},
		"identity": {"kernel": true}, "snapshot": {"kernel": true}, "kernel": {},
	}
	stats := compute(graph, nil, nil, nil, []string{"server", "snapshot/service", "identity", "snapshot", "kernel"})
	// server reaches 4, service reaches 3, identity/core each reach 1. Every
	// imported unit remains in the same reachability and n*(n-1) calculation.
	if got := propagationCost(stats); math.Abs(got-9.0/20.0) > 1e-12 {
		t.Fatalf("propagation arithmetic changed or an edge was dropped: %g", got)
	}
}
