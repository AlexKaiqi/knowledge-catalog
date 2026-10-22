package invocation

import "kc/snapshot"

func (s *observedVerifyingTree) VerifyTreeWrite(cs snapshot.TreeChangeSet, bulk bool) error {
	return s.observe("verify_write", treeChangeBytes(cs), func() error { return s.verifier.VerifyTreeWrite(cs, bulk) })
}
func (s *observedVerifyingObjectTree) VerifyTreeWrite(cs snapshot.TreeChangeSet, bulk bool) error {
	return s.observe("verify_write", treeChangeBytes(cs), func() error { return s.verifier.VerifyTreeWrite(cs, bulk) })
}
