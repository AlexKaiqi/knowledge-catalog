package main

import (
	"fmt"
	"math"
	"strings"
)

func hasInstabilityDrift(current, previous float64) bool {
	// Ce/(Ce+Ca) ratios may round a mathematical 0.05 boundary just below
	// its limit. This tolerance only tightens that existing hard threshold.
	return math.Abs(current-previous)+1e-12 >= 0.05
}

// Snapshot Store's HTTP adapter is an application capability inside the
// product's source directory. Its identity edge must not be attributed to the
// file/Dataset/access core. All other units keep the existing top-level policy.
const snapshotServiceUnit = "snapshot/service"

func unitPolicySignature(depth int) string {
	named := ""
	if depth == 1 {
		named = snapshotServiceUnit
	}
	return fmt.Sprintf("path-depth=%d;named-subtree=%s;version=1", depth, named)
}

func relativeUnit(path string, depth int) string {
	if depth == 1 && (path == snapshotServiceUnit || strings.HasPrefix(path, snapshotServiceUnit+"/")) {
		return snapshotServiceUnit
	}
	segments := strings.Split(path, "/")
	if len(segments) < depth {
		depth = len(segments)
	}
	return strings.Join(segments[:depth], "/")
}

func unitAt(module, importPath string, depth int) (string, bool) {
	path, ok := strings.CutPrefix(importPath, module+"/")
	if !ok {
		return "", false
	}
	return relativeUnit(path, depth), true
}

func comparableBaseline(base *baseline, depth int) error {
	policy := unitPolicySignature(depth)
	if base.Depth != depth || base.UnitPolicy != policy {
		return fmt.Errorf("unit policy mismatch: baseline depth=%d policy=%q, current depth=%d policy=%q; recompute both revisions with the same policy before tightening explicitly", base.Depth, base.UnitPolicy, depth, policy)
	}
	return nil
}
