package scale_test

import (
	"testing"

	"kc/kernel"
	"kc/scale"
)

func TestStarRocksStubFailsExplicitly(t *testing.T) {
	engine, err := scale.OpenStarRocks(scale.StarRocksConfig{})("", "kr://acme/core")
	if engine != nil || kernel.CodeOf(err) != kernel.ErrCapabilityUnsatisfied {
		t.Fatalf("stub must fail closed, engine=%#v err=%v", engine, err)
	}
}
