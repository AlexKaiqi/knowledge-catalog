package home

import (
	"testing"

	"kc/kernel"
	"kc/snapshot"
)

type capabilityPoisonStore struct{ snapshot.Store }

func (*capabilityPoisonStore) ID() kernel.RepositoryID { return "kr://poison/capability" }

func TestMissingProviderCapabilitiesFailWithoutPanic(t *testing.T) {
	source := &capabilityPoisonStore{}
	checks := []func() error{
		func() error { _, err := requireTreeCapability(source); return err },
		func() error { _, err := requireDirectoryCapability(source); return err },
		func() error { _, err := requireHistoryCapability(source); return err },
		func() error { _, err := requireChangeCapability(source); return err },
	}
	for _, check := range checks {
		if err := check(); kernel.CodeOf(err) != kernel.ErrCapabilityUnsatisfied {
			t.Fatalf("missing capability = %v", err)
		}
	}
}
