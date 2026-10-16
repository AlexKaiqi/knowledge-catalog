package invocation

import (
	"kc/invoke"
	"testing"
)

func TestDatasetDiscoveryEvidenceKeepsOperationAndConsumptionRuleSeparate(t *testing.T) {
	home := t.TempDir()
	if err := WriteAllow(home, AllowFile{Rules: []AllowRule{{ID: "consume", Principal: "reader", Dataset: "reports", Actions: []string{"file.read"}}}}); err != nil {
		t.Fatal(err)
	}
	flags := map[string]invoke.FlagValue{"_action": "file.read", "as": "reader", "dataset": "reports"}
	for command, action := range map[string]string{"object-search": "object.search", "object-relations": "object.relations", "object-traverse": "object.traverse"} {
		if got := ActionOf(command, flags); got != action {
			t.Fatalf("operation evidence used consumption action: %s -> %s", command, got)
		}
		if got := MatchedRuleID(home, command, flags); got != "consume" {
			t.Fatalf("operation %s lost its actual file.read rule: %s", command, got)
		}
		if _, err := RecordAudit(home, command, flags, nil, nil); err != nil {
			t.Fatal(err)
		}
		trail, err := ReadTrail(home, "kc", action, 10)
		if err != nil || len(trail) != 1 || trail[0].RuleID != "consume" || trail[0].Cmd != action {
			t.Fatalf("audit operation/rule evidence diverged: %#v %v", trail, err)
		}
	}
}
