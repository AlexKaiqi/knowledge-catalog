package kernel

import "errors"

// WriteOutcomeNotApplied is a positive proof, not a retry directive. An absent
// or unrecognized outcome makes no claim about authoritative publication.
const WriteOutcomeNotApplied = "NOT_APPLIED"

// IsWriteNotApplied recognizes the provider's proof through wrapped errors,
// or its preserved protocol representation. Error code and message alone are
// never evidence that a command did not publish.
func IsWriteNotApplied(err error) bool {
	if err == nil {
		return false
	}
	var proof interface{ WriteNotApplied() bool }
	if errors.As(err, &proof) && proof.WriteNotApplied() {
		return true
	}
	fault := AsIngress(err)
	return fault != nil && fault.WriteOutcome == WriteOutcomeNotApplied
}
