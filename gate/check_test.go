package gate_test

import (
	"testing"

	"kc/gate"
	"kc/internal/testkit"
	"kc/kernel"
)

func TestCheckEmptyRequired(t *testing.T) {
	if err := gate.Check(nil, nil); err != nil {
		t.Fatal(err)
	}
}

func TestCheckValidateAcceptsStructure(t *testing.T) {
	err := gate.Check([]string{"validate"}, []gate.Evidence{
		{Name: "structure", BasisID: "G1", Outcome: "PASSED"},
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestCheckSuiteName(t *testing.T) {
	err := gate.Check([]string{"suite:metrics-contract"}, []gate.Evidence{
		{Name: "metrics-contract", BasisID: "G1", Outcome: "PASSED"},
	})
	if err != nil {
		t.Fatal(err)
	}
	err = gate.Check([]string{"suite:metrics-contract"}, []gate.Evidence{
		{Name: "metrics-contract", BasisID: "G1", Outcome: "FAILED"},
	})
	testkit.ExpectCode(t, err, kernel.ErrGateUnsatisfied)
}

func TestCheckMissing(t *testing.T) {
	err := gate.Check([]string{"validate", "suite:lint"}, []gate.Evidence{
		{Name: "structure", BasisID: "G1", Outcome: "PASSED"},
	})
	testkit.ExpectCode(t, err, kernel.ErrGateUnsatisfied)
}

func TestCheckOnBasis(t *testing.T) {
	got := gate.OnBasis([]gate.Evidence{
		{Name: "lint", BasisID: "G1", Outcome: "PASSED"},
		{Name: "lint", BasisID: "G2", Outcome: "PASSED"},
	}, "G1")
	if len(got) != 1 || got[0].BasisID != "G1" {
		t.Fatal(got)
	}
}

func TestCheckValidateAcceptsValidateName(t *testing.T) {
	if err := gate.Check([]string{"validate"}, []gate.Evidence{
		{Name: "validate", BasisID: "G1", Outcome: "PASSED"},
	}); err != nil {
		t.Fatal(err)
	}
}

func TestCheckSuitePrefixOnEvidence(t *testing.T) {
	if err := gate.Check([]string{"suite:approval:steward"}, []gate.Evidence{
		{Name: "suite:approval:steward", BasisID: "G1", Outcome: "PASSED"},
	}); err != nil {
		t.Fatal(err)
	}
}

func TestCheckIgnoresFailedEvenIfNameMatches(t *testing.T) {
	err := gate.Check([]string{"validate"}, []gate.Evidence{
		{Name: "structure", BasisID: "G1", Outcome: "FAILED"},
		{Name: "lint", BasisID: "G1", Outcome: "PASSED"},
	})
	testkit.ExpectCode(t, err, kernel.ErrGateUnsatisfied)
}

func TestParseAndValidateRequire(t *testing.T) {
	got := gate.ParseRequire("validate, suite:lint, validate, ,suite:approval:steward")
	if len(got) != 3 || got[0] != "validate" || got[1] != "suite:lint" || got[2] != "suite:approval:steward" {
		t.Fatal(got)
	}
	testkit.ExpectCode(t, gate.ValidateRequire(nil), kernel.ErrPreconditionFailed)
	testkit.ExpectCode(t, gate.ValidateRequire([]string{"lint"}), kernel.ErrPreconditionFailed)
	testkit.ExpectCode(t, gate.ValidateRequire([]string{"suite:"}), kernel.ErrPreconditionFailed)
	if err := gate.ValidateRequire([]string{"validate", "suite:lint"}); err != nil {
		t.Fatal(err)
	}
	testkit.ExpectCode(t, gate.ValidateOn("put"), kernel.ErrPreconditionFailed)
	if err := gate.ValidateOn(gate.OnMerge); err != nil {
		t.Fatal(err)
	}
}

func TestRequiredUnionsMatchingRules(t *testing.T) {
	file := gate.File{Rules: []gate.Rule{
		{On: gate.OnMerge, Repo: "kr://acme/semantic", Require: []string{"validate"}},
		{On: gate.OnMerge, Repo: "kr://acme/semantic", Require: []string{"suite:lint"}},
		{On: gate.OnMerge, Repo: "kr://acme/physical", Require: []string{"suite:other"}},
		{On: gate.OnPromote, Catalog: "kr://acme/catalog", Release: "stable", Require: []string{"suite:contract"}},
	}}
	got := file.Required(gate.OnMerge, "kr://acme/semantic", "", "")
	if len(got) != 2 || got[0] != "validate" || got[1] != "suite:lint" {
		t.Fatal(got)
	}
	if n := file.Required(gate.OnPromote, "", "kr://acme/catalog", "stable"); len(n) != 0 {
		t.Fatal(n)
	}
}
