package scale_test

import (
	"testing"

	"kc/internal/testkit"
	"kc/kernel"
	"kc/repository"
	"kc/scale"
)

func TestScaleStubsFailExplicitly(t *testing.T) {
	stream, err := scale.OpenStream("unused", "kr://acme/public/core")
	if err != nil {
		t.Fatal(err)
	}
	testkit.ExpectCode(t, repository.CheckStreamAvailable(stream), kernel.ErrCapabilityUnsatisfied)
	_, err = stream.Append("events", []repository.AppendEntry{{EventID: "e1", Payload: 1}}, "")
	testkit.ExpectCode(t, err, kernel.ErrCapabilityUnsatisfied)

	opener := scale.OpenStarRocks(scale.StarRocksConfig{})
	_, err = opener(t.TempDir(), "kr://acme/public/core")
	testkit.ExpectCode(t, err, kernel.ErrCapabilityUnsatisfied)
}
