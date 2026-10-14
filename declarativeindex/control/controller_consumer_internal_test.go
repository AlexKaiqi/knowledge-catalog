package control

import (
	fixtures "kc/declarativeindex/internal/testfixture"
	ports "kc/discovery/projection"
	sourcebridge "kc/liveaccess/discovery"
	"path/filepath"
	"testing"

	"kc/kernel"
)

func TestConsumerOnlyControllerRejectsStateNoticeWithRuntime(t *testing.T) {
	c, err := NewController(nil, NewTargetStore(filepath.Join(t.TempDir(), "controller.db")), nil)
	if err != nil {
		t.Fatal(err)
	}
	c.SetObservationSource(sourcebridge.Source{Lookup: &fixtures.StateLookup{}})
	if err := c.Notify(ports.ChangeNotice{Repository: "kr://acme/public/core"}); kernel.CodeOf(err) != kernel.ErrCapabilityUnsatisfied {
		t.Fatalf("notice err=%v", err)
	}
	notices, err := c.store.pendingNotices()
	if err != nil || len(notices) != 0 {
		t.Fatalf("unsupported State notice was enqueued: %#v err=%v", notices, err)
	}
}
