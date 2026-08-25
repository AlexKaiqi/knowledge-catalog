package starrocks_test

import (
	"testing"

	"kc/kernel"
	"kc/retrieval/starrocks"
)

func TestStarRocksStubFailsExplicitly(t *testing.T) {
	engine, err := starrocks.Open(starrocks.Config{})("", "kr://acme/core")
	if engine != nil || kernel.CodeOf(err) != kernel.ErrCapabilityUnsatisfied {
		t.Fatalf("stub must fail closed, engine=%#v err=%v", engine, err)
	}
}
