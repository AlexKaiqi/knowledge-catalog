package cli

import (
	"regexp"
	"slices"
	"strings"
	"testing"
)

// helpVerb matches the "  kc <verb>" lines that document a callable verb.
var helpVerb = regexp.MustCompile(`(?m)^\s+kc ([a-z][a-z0-9-]*)`)

// serve is the one verb that is not in the table: it does not return a result,
// so Run handles it before dispatch.
var notDispatched = []string{"serve"}

func documentedVerbs() []string {
	seen := map[string]bool{}
	var out []string
	for _, m := range helpVerb.FindAllStringSubmatch(Help, -1) {
		if !seen[m[1]] {
			seen[m[1]] = true
			out = append(out, m[1])
		}
	}
	slices.Sort(out)
	return out
}

func TestEveryDocumentedVerbIsRegistered(t *testing.T) {
	for _, verb := range documentedVerbs() {
		if slices.Contains(notDispatched, verb) {
			continue
		}
		if !Verb(verb) {
			t.Errorf("kc %s appears in Help but is in no verbs_*.go table", verb)
		}
	}
}

func TestEveryRegisteredVerbIsDocumented(t *testing.T) {
	documented := documentedVerbs()
	for verb := range commands {
		if !slices.Contains(documented, verb) {
			t.Errorf("verb %s is registered but absent from Help", verb)
		}
	}
}

// TestRemovedVerbsAreNotRegistered keeps the old composition vocabulary out of
// both the command table and dispatch.
func TestRemovedVerbsAreNotRegistered(t *testing.T) {
	for _, verb := range []string{"mount", "define-view", "retire-view", "read-view", "read-catalog", "read-release", "pin-view", "promote", "rollback", "retire-release", "append", "stream", "index-plan"} {
		if Verb(verb) {
			t.Errorf("retired verb %s is registered again", verb)
		}
		_, err := dispatch(verb, map[string]FlagValue{"home": t.TempDir()})
		if err == nil || !strings.Contains(err.Error(), "unknown command "+verb) {
			t.Errorf("dispatch(%s) should reject the removed command, got %v", verb, err)
		}
	}
}

func TestRemovedCompositionFlagsAreRejected(t *testing.T) {
	for _, flag := range []string{"view", "release", "generation", "base-generation", "input-vrv"} {
		_, err := dispatch("read", map[string]FlagValue{"home": t.TempDir(), flag: "old"})
		if err == nil || !strings.Contains(err.Error(), "unknown flag --"+flag) {
			t.Errorf("--%s should be rejected before opening a home, got %v", flag, err)
		}
	}
}

func TestUnknownVerbIsUsageInvalid(t *testing.T) {
	result := Invoke("frobnicate", map[string]FlagValue{"home": t.TempDir()})
	if result.Status == 0 {
		t.Fatal("unknown verb must fail")
	}
	if !strings.Contains(result.Stdout, "USAGE_INVALID") {
		t.Fatal(result.Stdout)
	}
}

// TestGovernedVerbsNeedAWorkspace pins the stage wiring: a protocol verb must
// not silently succeed against a home that was never initialised.
func TestGovernedVerbsNeedAWorkspace(t *testing.T) {
	home := t.TempDir()
	for _, verb := range []string{"put", "read", "search", "propose", "define-workspace"} {
		if commands[verb].stage != stageGoverned {
			t.Errorf("%s should be stageGoverned", verb)
		}
		if _, err := dispatch(verb, map[string]FlagValue{"home": home}); err == nil {
			t.Errorf("%s on an uninitialised home should fail", verb)
		}
	}
}

// TestHomeStageVerbsSkipMounting is the reason audit and the grant verbs are at
// stageHome: they must answer even when no workspace can be opened.
func TestHomeStageVerbsSkipMounting(t *testing.T) {
	for _, verb := range []string{"audit", "whoami", "allowed", "hook-ls", "gate-ls"} {
		if commands[verb].stage != stageHome {
			t.Errorf("%s should be stageHome", verb)
		}
	}
	result := Invoke("whoami", map[string]FlagValue{"home": t.TempDir()})
	if result.Status != 0 || !strings.Contains(result.Stdout, "owner") {
		t.Fatal(result.Stdout)
	}
}

func TestLimitFlagRejectsGarbageOnEveryVerb(t *testing.T) {
	for _, raw := range []string{"abc", "-1", "1.5"} {
		if _, err := limitFrom(map[string]FlagValue{"limit": raw}, 50); err == nil {
			t.Errorf("--limit %s should be rejected", raw)
		}
	}
	got, err := limitFrom(map[string]FlagValue{}, 50)
	if err != nil || got != 50 {
		t.Fatalf("absent --limit should fall back: %d %v", got, err)
	}
	got, err = limitFrom(map[string]FlagValue{"limit": "7"}, 50)
	if err != nil || got != 7 {
		t.Fatalf("--limit 7: %d %v", got, err)
	}
}
