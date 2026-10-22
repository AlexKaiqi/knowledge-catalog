package invocation

import (
	"kc/kernel"
	objectmodel "kc/object"
	"kc/snapshot"
	"strconv"
)

// Restore issued coordinates before authorization or target resolution. A
// continuation supplies no grant; normal dispatch and source checks still run.
func prepareHistoryContinuation(cx *Invocation, action string) error {
	if cx.Command != "object-log" || cx.Flag("continuation") == "" {
		return nil
	}
	if action != "object.history.read" {
		return kernel.Fail(kernel.ErrUsageInvalid, "history continuation requires the object history action")
	}
	if cx.WS.HistoryCheckpoints == nil {
		return kernel.Fail(kernel.ErrCapabilityUnsatisfied, "durable history continuations are unavailable")
	}
	checkpoint, err := cx.WS.HistoryCheckpoints.Resolve(cx.Context, cx.Flag("continuation"))
	if err != nil {
		return err
	}
	ref, _ := objectmodel.ParseReference(checkpoint.Basis.Reference)
	if supplied := cx.Flag("object"); supplied != "" && supplied != string(ref.Object) {
		return kernel.Fail(kernel.ErrUsageInvalid, "history continuation names a different object")
	}
	for _, name := range []string{"pin", "dataset-file", "source"} {
		if cx.Flag(name) != "" {
			return kernel.Fail(kernel.ErrUsageInvalid, "history continuation already fixes its scope; do not add --%s", name)
		}
	}
	if SuppliedDataset(cx.Flags) != nil {
		return kernel.Fail(kernel.ErrUsageInvalid, "history continuation already retains its temporary definition")
	}
	basis := checkpoint.Basis
	if basis.Dataset != "" || basis.Temporary {
		if cx.Flag("repo") != "" || cx.Flag("commit") != "" || cx.Flag("ref") != "" {
			return kernel.Fail(kernel.ErrUsageInvalid, "history continuation fixes a Dataset scope")
		}
		if supplied := cx.Flag("dataset"); supplied != "" && supplied != basis.Dataset {
			return kernel.Fail(kernel.ErrUsageInvalid, "history continuation names a different Dataset")
		}
		if supplied := cx.Flag("revision"); supplied != "" {
			revision, err := strconv.Atoi(supplied)
			if err != nil || revision != basis.Revision {
				return kernel.Fail(kernel.ErrUsageInvalid, "history continuation fixes a different Dataset revision")
			}
		}
		if basis.Temporary {
			local := basis
			local.Dependencies = nil
			local.UnresolvedDependencies = nil
			if err := restoreTemporaryReadBasis(cx, local); err != nil {
				return err
			}
		} else {
			cx.Flags["dataset"], cx.Flags["revision"] = basis.Dataset, strconv.Itoa(basis.Revision)
		}
	} else {
		if len(basis.Candidates) != 1 || cx.Flag("dataset") != "" || cx.Flag("revision") != "" {
			return kernel.Fail(kernel.ErrUsageInvalid, "history continuation fixes one Repository scope")
		}
		source := basis.Candidates[0]
		if cx.Flag("repo") != "" && cx.Flag("repo") != string(source.Repository) || cx.Flag("commit") != "" && cx.Flag("commit") != string(source.Commit) || cx.Flag("ref") != "" && snapshot.RefOrDefault(cx.Flag("ref")) != checkpoint.Ref {
			return kernel.Fail(kernel.ErrUsageInvalid, "history continuation fixes different Repository coordinates")
		}
		cx.Flags["repo"], cx.Flags["commit"], cx.Flags["ref"] = string(source.Repository), string(source.Commit), checkpoint.Ref
	}
	cx.Flags["object"] = string(ref.Object)
	cx.historyCheckpoint = &checkpoint
	return nil
}
