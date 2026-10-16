package home

import "strings"

// managedLegacyActionPrefix is the persisted spelling written by older releases.
// Its only use is validating a frozen allocation policy during recovery.
const managedLegacyActionPrefix = "knowledge."

// managedReceiptActions validates a completed historical policy with the current
// closed set. Pending allocations retain strict current spelling: a retry must
// never send retired actions to the grant callback and mark them READY.
// Decoding retains the original actions and digest without migrating receipts.
func managedReceiptActions(actions []string, phase string) []string {
	if phase != managedReady {
		return actions
	}
	current := make([]string, 0, len(actions))
	retiredSearch := false
	for _, action := range actions {
		if action == "object.search" || action == managedLegacyActionPrefix+"search" {
			if retiredSearch {
				return []string{"invalid-duplicate-retired-search"}
			}
			retiredSearch = true
			continue
		}
		switch action {
		case managedLegacyActionPrefix + "read", managedLegacyActionPrefix + "schema.read", managedLegacyActionPrefix + "provenance", managedLegacyActionPrefix + "history.read", managedLegacyActionPrefix + "relations":
			action = "object." + strings.TrimPrefix(action, managedLegacyActionPrefix)
		}
		current = append(current, action)
	}
	// SEARCH grants have no current effect. Validation of a completed receipt
	// retains their historical admissibility without regranting or rewriting it.
	if len(current) == 0 && retiredSearch {
		current = append(current, "object.read")
	}
	return current
}
