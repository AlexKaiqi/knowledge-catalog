package arch_test

import "testing"

func TestDatasetSpaceHasAnExplicitDiscoveryLayerAndNoRuntimeDependencies(t *testing.T) {
	if layer, registered := architectureLayer("discovery/space"); !registered || layer != "discovery" {
		t.Fatal("the fixed Dataset namespace must be an explicit Discovery capability")
	}
	if _, registered := architectureLayer("discovery/space/internal/future"); registered {
		t.Fatal("Dataset namespace children must not inherit permission by prefix")
	}
	var denied []string
	for _, rule := range forbidden {
		if rule.pkg == "discovery/space" {
			denied = rule.denied
		}
	}
	if len(denied) == 0 {
		t.Fatal("Dataset namespace has no dependency boundary")
	}
	for _, target := range []string{"object/reader", "object/maintenance", "object/writer", "server", "liveaccess", "declarativeindex/projection", "discovery/projection"} {
		t.Run(target, func(t *testing.T) {
			graph := depGraph{"discovery/space": {"scope-bridge"}, "scope-bridge": {target}}
			found := false
			for _, boundary := range denied {
				if _, violation := reachDenied(graph.reachable("discovery/space"), boundary); violation {
					found = true
				}
			}
			want := target != "object/reader" && target != "object/maintenance"
			if found != want {
				t.Fatalf("namespace dependency %s: forbidden=%v want=%v", target, found, want)
			}
		})
	}
}

func TestDatasetAssemblyDelegatesNamespaceAndPinConversion(t *testing.T) {
	for _, test := range []struct{ source, target string }{
		{"discovery/plugin", "discovery/space"},
		{"discovery/datasets", "object/reader"},
		{"discovery/datasets", "object/maintenance"},
		{"discovery/datasets", "object/writer"},
	} {
		t.Run(test.source+"/"+test.target, func(t *testing.T) {
			graph := depGraph{test.source: {"assembly-bridge"}, "assembly-bridge": {test.target}}
			for _, rule := range forbidden {
				if rule.pkg != test.source {
					continue
				}
				for _, boundary := range rule.denied {
					if _, violation := reachDenied(graph.reachable(test.source), boundary); violation {
						return
					}
				}
			}
			t.Fatalf("assembly can copy namespace or pin conversion through %s", test.target)
		})
	}
}
