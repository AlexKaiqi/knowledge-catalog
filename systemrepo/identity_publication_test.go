package systemrepo_test

import (
	"regexp"
	"strings"
	"testing"

	objectmodel "kc/object"
	"kc/systemrepo"
)

// These assertions guard the retired claims that contradicted the public URN
// and holding contracts. They apply to every reachable publication body, not
// just the three pages where the contradiction was first found.
func TestSystemPublicationDoesNotQualifyObjectIdentityByRepository(t *testing.T) {
	retired := []string{
		"身份由来源 Repository 与 object_id 确定",
		"不同仓的同名对象仍是不同对象",
		"URN 编码来源仓",
		"URN 指认来源 Repository",
		"形成对象身份、Snapshot",
	}
	urns := regexp.MustCompile("`(urn:kc:object:[^`]+)`")
	checked := 0
	for _, operation := range systemrepo.SystemPublicationOperations() {
		value, ok := operation.Value.(map[string]any)
		if !ok {
			continue
		}
		body, _ := value["body"].(string)
		for _, claim := range retired {
			if strings.Contains(body, claim) {
				t.Errorf("%s contradicts repository-independent identity: %s", operation.PathHint, claim)
			}
		}
		for _, match := range urns.FindAllStringSubmatch(body, -1) {
			ref, err := objectmodel.ParseReference(match[1])
			if err != nil {
				t.Errorf("%s publishes an invalid object reference %q: %v", operation.PathHint, match[1], err)
				continue
			}
			ref.Repository = "kr://moved/source"
			moved, err := objectmodel.FormatReference(ref)
			if err != nil || moved != match[1] {
				t.Errorf("%s example changes identity with its repository: %q -> %q (%v)", operation.PathHint, match[1], moved, err)
			}
			checked++
		}
	}
	if checked == 0 {
		t.Fatal("system publication has no concrete object URN examples")
	}
}
