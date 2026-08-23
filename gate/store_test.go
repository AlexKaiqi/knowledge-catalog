package gate_test

import (
	"testing"

	"kc/gate"
	"kc/internal/testkit"
)

func TestReadMissingFileIsEmpty(t *testing.T) {
	home := testkit.TempDir(t)
	file, err := gate.Read(home)
	if err != nil || len(file.Rules) != 0 {
		t.Fatal(file, err)
	}
}

func TestWriteReadRoundTrip(t *testing.T) {
	home := testkit.TempDir(t)
	want := gate.File{Rules: []gate.Rule{{
		ID: "gt_1", On: gate.OnMerge, Repo: "kr://acme/semantic", Require: []string{"validate", "suite:lint"},
	}}}
	if err := gate.Write(home, want); err != nil {
		t.Fatal(err)
	}
	got, err := gate.Read(home)
	if err != nil || len(got.Rules) != 1 || got.Rules[0].ID != "gt_1" || got.Rules[0].Require[1] != "suite:lint" {
		t.Fatal(got, err)
	}
}

func TestRequiredMergeOtherRepoEmpty(t *testing.T) {
	file := gate.File{Rules: []gate.Rule{
		{On: gate.OnMerge, Repo: "kr://acme/semantic", Require: []string{"suite:lint"}},
	}}
	if got := file.Required(gate.OnMerge, "kr://acme/physical", ""); len(got) != 0 {
		t.Fatal(got)
	}
}
