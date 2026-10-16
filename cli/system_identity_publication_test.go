package cli_test

import (
	"encoding/json"
	"reflect"
	"regexp"
	"testing"

	"kc/cli"
	objectmodel "kc/object"
)

// Read the publication through the authenticated application boundary. The
// source-level publication guard remains responsible for all publication bodies;
// this contract also checks that scope and fixed evidence stay outside the URN
// in the response a new reader actually receives.
func TestPublicSystemIdentityExplanationKeepsScopeOutsideURN(t *testing.T) {
	d := openExampleDeployment(t, exampleWorld{})
	root := cli.Run([]string{"help"})
	if root.Status != 0 {
		t.Fatal(root)
	}
	if !publicHoldingConflictExplanation(root.Stdout) {
		t.Fatal("root help must explain same-Aspect conflicts from multiple repositories in a read scope, including OBJECT_ID_CONFLICT")
	}
	texts := map[string]string{"root-help": root.Stdout}
	responses := map[string]any{}
	for _, concept := range []string{"object", "address", "repository"} {
		reference := "urn:kc:object:kc/concept/" + concept + "#/definition"
		read := asMap(t, body(t, d.as(t, "agent:identity-reader", "read", reference, "--repo", string(objectmodel.SystemRepositoryID))))
		commit, _ := read["commit"].(string)
		if read["reference"] != reference || read["repository"] != string(objectmodel.SystemRepositoryID) || commit == "" {
			t.Fatalf("%s must keep the unqualified reference and a separate fixed source: %#v", concept, read)
		}
		if read["objectId"] != "kc/concept/"+concept {
			t.Fatalf("%s changed object identity at the public boundary: %#v", concept, read)
		}
		fixed := asMap(t, body(t, d.as(t, "agent:identity-reader", "read", reference,
			"--repo", string(objectmodel.SystemRepositoryID), "--commit", commit)))
		if fixed["reference"] != reference || fixed["commit"] != commit || !reflect.DeepEqual(fixed["value"], read["value"]) {
			t.Fatalf("%s cannot be reproduced from its separate source basis: %#v", concept, fixed)
		}
		texts[concept], _ = asMap(t, read["value"])["body"].(string)
		if texts[concept] == "" {
			t.Fatalf("%s has no explanation in the public response", concept)
		}
		responses[concept] = read
	}
	for name, text := range texts {
		if claim := publicIdentityContradiction(text); claim != "" {
			t.Errorf("%s contradicts repository-independent object identity: %q", name, claim)
		}
	}
	checkPublicIdentityReferences(t, texts)
	evidence, err := json.Marshal(map[string]any{"root_help": root.Stdout, "responses": responses})
	if err != nil {
		t.Fatal(err)
	}
	// A retained verbose test log is also a self-contained public-only input for
	// a fresh concept-reading session; it contains no source files or prompts.
	t.Logf("PUBLIC_IDENTITY_RESPONSE=%s", evidence)
}

var publicIdentityRetiredRules = []*regexp.Regexp{
	regexp.MustCompile(`(?:身份|URN)[^。\n；]*(?:来源\s*Repository\s*与\s*object_id|(?:编码|指认)[^。\n；]*(?:来源仓|来源\s*Repository))`),
	regexp.MustCompile(`不同仓[^。\n；]*同名对象[^。\n；]*不同对象`),
	regexp.MustCompile(`形成对象身份[、，][^。\n；]*(?:Snapshot|版本)`),
}

func publicIdentityContradiction(text string) string {
	for _, rule := range publicIdentityRetiredRules {
		if match := rule.FindString(text); match != "" {
			return match
		}
	}
	return ""
}

func TestPublicIdentityExplanationRejectsOriginalContradictions(t *testing.T) {
	for _, claim := range []string{
		"身份由来源 Repository 与 object_id 确定",
		"不同仓的同名对象仍是不同对象",
		"对象 URN 编码来源仓与 object_id",
		"URN 指认来源 Repository",
		"形成对象身份、Snapshot 版本图、历史、来源与维护权威的边界",
	} {
		if publicIdentityContradiction(claim) == "" {
			t.Errorf("guard accepted an original contradictory explanation: %q", claim)
		}
	}
}

func checkPublicIdentityReferences(t *testing.T, texts map[string]string) {
	t.Helper()
	urns := regexp.MustCompile("urn:kc:object:[^\\s'`\"]+")
	checked := 0
	for name, text := range texts {
		for _, urn := range urns.FindAllString(text, -1) {
			ref, err := objectmodel.ParseReference(urn)
			if err != nil {
				t.Fatalf("%s publishes an invalid object reference %q: %v", name, urn, err)
			}
			if ref.Repository != "" {
				t.Errorf("%s puts read scope in object identity: %q", name, urn)
			}
			ref.Repository = "kr://identity-test/moved"
			moved, err := objectmodel.FormatReference(ref)
			if err != nil || moved != urn {
				t.Errorf("%s example changes its identity after a repository move: %q -> %q (%v)", name, urn, moved, err)
			}
			checked++
		}
	}
	if checked < 3 {
		t.Fatal("public root help and object/address explanations must retain concrete identity examples")
	}
}

// The error code alone is insufficient: a reader needs the scope, unit and
// competing holders that cause it. Accept equivalent terminology rather than
// prescribing a sentence from today's help text.
func publicHoldingConflictExplanation(text string) bool {
	rules := []*regexp.Regexp{
		regexp.MustCompile(`OBJECT_ID_CONFLICT`),
		regexp.MustCompile(`(?:同一|相同)[^。\n；]*(?:Aspect|方面|切面)`),
		regexp.MustCompile(`(?:两|多|不同)[^。\n；]*仓`),
		regexp.MustCompile(`范围|scope`),
	}
	for _, line := range regexp.MustCompile(`[。\n]`).Split(text, -1) {
		complete := true
		for _, rule := range rules {
			complete = complete && rule.MatchString(line)
		}
		if complete {
			return true
		}
	}
	return false
}

func TestPublicConflictExplanationRequiresHoldingContext(t *testing.T) {
	for _, incomplete := range []string{
		"同一 object_id 在多个仓仍是同一对象。",
		"OBJECT_ID_CONFLICT 表示对象冲突。",
		"同一范围内同一 Aspect 可以由两个仓持有。",
		"同一对象在不同仓出现时返回 OBJECT_ID_CONFLICT。",
	} {
		if publicHoldingConflictExplanation(incomplete) {
			t.Errorf("incomplete explanation passes the public contract: %q", incomplete)
		}
	}
	if !publicHoldingConflictExplanation("在请求范围内，相同方面同时存在于不同来源仓时返回 OBJECT_ID_CONFLICT。") {
		t.Error("equivalent explanation must pass without using the current help sentence")
	}
}
