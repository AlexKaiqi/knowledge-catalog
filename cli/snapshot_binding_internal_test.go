package cli

import (
	"reflect"
	"testing"
)

func TestSnapshotPositionalsBindModuleDeclarationsWithoutOpeningInputs(t *testing.T) {
	for _, row := range []struct {
		handler string
		args    []string
		bound   map[string]FlagValue
	}{
		{"attach", []string{"repo-A"}, map[string]FlagValue{"repo": "repo-A"}},
		{"detach", []string{"repo-A"}, map[string]FlagValue{"repo": "repo-A"}},
		{"dataset-define", []string{"delivery"}, map[string]FlagValue{"dataset": "delivery"}},
		{"dataset-show", []string{"delivery"}, map[string]FlagValue{"dataset": "delivery"}},
		{"dataset-retire", []string{"delivery"}, map[string]FlagValue{"dataset": "delivery"}},
		{"dataset-clone", []string{"delivery", "missing-local-output"}, map[string]FlagValue{"dataset": "delivery", "dir": "missing-local-output"}},
		{"dataset-clone", []string{"delivery"}, map[string]FlagValue{"dataset": "delivery"}},
	} {
		t.Run(row.handler+"/"+row.args[len(row.args)-1], func(t *testing.T) {
			// Binding is syntactic. No valid recipe, semantic input, client or
			// source authority is necessary, and host context stays unchanged.
			flags := map[string]FlagValue{"as": "caller", "request-id": "correlation", "file": "missing-recipe"}
			wanted := map[string]FlagValue{"as": "caller", "request-id": "correlation", "file": "missing-recipe"}
			for name, value := range row.bound {
				wanted[name] = value
			}
			if err := applyPositionals(row.handler, flags, row.args); err != nil {
				t.Fatalf("module operands required more than argument binding: %v", err)
			}
			if !reflect.DeepEqual(flags, wanted) {
				t.Fatalf("binding changed module operands or host context: %#v want %#v", flags, wanted)
			}
		})
	}
}

func TestSnapshotPositionalsRejectConflictsAndExtraValuesExactly(t *testing.T) {
	for _, row := range []struct {
		handler string
		flags   map[string]FlagValue
		args    []string
		message string
	}{
		{"attach", map[string]FlagValue{"repo": "already-selected"}, []string{"repo-A"}, "unexpected argument repo-A"},
		{"detach", map[string]FlagValue{"repo": "already-selected"}, []string{"repo-A"}, "unexpected argument repo-A"},
		{"dataset-define", map[string]FlagValue{"dataset": "already-selected"}, []string{"delivery"}, "unexpected argument delivery"},
		{"dataset-show", map[string]FlagValue{"dataset": "already-selected"}, []string{"delivery"}, "unexpected argument delivery"},
		{"dataset-retire", map[string]FlagValue{"dataset": "already-selected"}, []string{"delivery"}, "unexpected argument delivery"},
		{"dataset-clone", map[string]FlagValue{"dataset": "already-selected"}, []string{"delivery", "target"}, "unexpected argument delivery"},
		{"dataset-clone", map[string]FlagValue{"dir": "already-selected"}, []string{"delivery", "target"}, "unexpected argument target"},
		{"attach", map[string]FlagValue{}, []string{"repo-A", "extra"}, "unexpected argument extra"},
		{"detach", map[string]FlagValue{}, []string{"repo-A", "extra"}, "unexpected argument extra"},
		{"dataset-define", map[string]FlagValue{}, []string{"delivery", "extra"}, "unexpected argument extra"},
		{"dataset-show", map[string]FlagValue{}, []string{"delivery", "extra"}, "unexpected argument extra"},
		{"dataset-retire", map[string]FlagValue{}, []string{"delivery", "extra"}, "unexpected argument extra"},
		{"dataset-clone", map[string]FlagValue{}, []string{"delivery", "target", "extra"}, "unexpected argument extra"},
		{"files-read", map[string]FlagValue{"dataset": "delivery", "path": "file.bin"}, []string{"extra"}, "unexpected argument extra"},
	} {
		t.Run(row.handler+"/"+row.message, func(t *testing.T) {
			if err := applyPositionals(row.handler, row.flags, row.args); err == nil || err.Error() != row.message {
				t.Fatalf("conflict/extra operand changed its exact failure: %v want %q", err, row.message)
			}
		})
	}
}
