package discovery_test

import (
	"strings"
	"testing"

	retrieval "kc/discovery"
	"kc/kernel"
	objectmodel "kc/object"
)

// A field that only an unsupported member declares has no searchable face; the
// refusal names that member and its reason instead of calling the field absent.
func TestFieldDeclaredOnlyByUnreadyMemberNamesTheMember(t *testing.T) {
	member := objectmodel.Address{Kind: "Member", ObjectID: "index/fixture", AspectName: "index", MemberKey: "runtime-value"}
	spec := retrieval.AccessSpec{Declarations: []retrieval.DeclarationStatus{{
		Member: member,
		Status: retrieval.DeclarationUnsupported,
		Reason: "runtime-value needs freshness-lag and per-caller handling",
		Inputs: []retrieval.FieldRef{{Schema: "schema/table.runtime", Aspect: "runtime", Path: "value"}},
	}}}
	_, err := spec.ResolveField(retrieval.FieldRef{Schema: "schema/table.runtime", Aspect: "runtime", Path: "value"})
	if kernel.CodeOf(err) != kernel.ErrCapabilityUnsatisfied {
		t.Fatalf("err = %v, want CAPABILITY_UNSATISFIED", err)
	}
	message := err.Error()
	for _, want := range []string{objectmodel.AddressLabel(member), "unsupported", "freshness-lag", "per-caller"} {
		if !strings.Contains(message, want) {
			t.Fatalf("message %q lacks %q", message, want)
		}
	}
	if strings.Contains(message, "\x1f") {
		t.Fatalf("message %q leaks an internal key", message)
	}

	_, err = spec.ResolveField(retrieval.FieldRef{Schema: "schema/table.runtime", Path: "other"})
	if kernel.CodeOf(err) != kernel.ErrCapabilityUnsatisfied || !strings.Contains(err.Error(), "not in AccessSpec") {
		t.Fatalf("undeclared field err = %v, want not in AccessSpec", err)
	}
}
