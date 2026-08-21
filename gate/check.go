package gate

import (
	"strings"

	"kc/kernel"
)

const (
	OnMerge         = "merge"
	OnPromote       = "promote"
	RequireValidate = "validate"
	SuitePrefix     = "suite:"
	StructureSuite  = "structure"
)

func OnBasis(got []Evidence, basisID string) []Evidence {
	out := []Evidence{}
	for _, item := range got {
		if item.BasisID == basisID {
			out = append(out, item)
		}
	}
	return out
}

// Evidence is a PASSED/FAILED record bound to a Preview or Generation id.
type Evidence struct {
	Name    string
	BasisID string
	Outcome string
}

func Check(required []string, got []Evidence) error {
	if len(required) == 0 {
		return nil
	}
	for _, name := range required {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		if !passed(name, got) {
			return kernel.Fail(kernel.ErrGateUnsatisfied, "gate %s is not PASSED on this basis", name)
		}
	}
	return nil
}

func passed(required string, got []Evidence) bool {
	for _, item := range got {
		if item.Outcome != "PASSED" {
			continue
		}
		if nameMatch(required, item.Name) {
			return true
		}
	}
	return false
}

func nameMatch(required, evidence string) bool {
	required = strings.TrimSpace(required)
	evidence = strings.TrimSpace(evidence)
	if required == RequireValidate {
		return evidence == RequireValidate || evidence == StructureSuite
	}
	if strings.HasPrefix(required, SuitePrefix) {
		want := strings.TrimPrefix(required, SuitePrefix)
		return evidence == want || evidence == required
	}
	return required == evidence
}

func ParseRequire(raw string) []string {
	out := []string{}
	seen := map[string]bool{}
	for _, part := range strings.Split(raw, ",") {
		part = strings.TrimSpace(part)
		if part == "" || seen[part] {
			continue
		}
		seen[part] = true
		out = append(out, part)
	}
	return out
}

func ValidateRequire(names []string) error {
	if len(names) == 0 {
		return kernel.Fail(kernel.ErrPreconditionFailed, "gate requires --require")
	}
	for _, name := range names {
		if name == RequireValidate {
			continue
		}
		if strings.HasPrefix(name, SuitePrefix) && strings.TrimPrefix(name, SuitePrefix) != "" {
			continue
		}
		return kernel.Fail(kernel.ErrPreconditionFailed, "unknown gate check %s (use validate or suite:<name>)", name)
	}
	return nil
}
