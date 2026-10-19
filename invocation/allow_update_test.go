package invocation

import (
	"fmt"
	"sync"
	"testing"
)

// Grant, revoke, share and provisioning all derive the next policy inside
// UpdateAllow; concurrent updates on one served root must not drop each other.
func TestConcurrentAllowUpdatesKeepEveryRule(t *testing.T) {
	home := t.TempDir()
	const grants = 16
	var wg sync.WaitGroup
	errs := make(chan error, grants)
	for i := 0; i < grants; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			errs <- UpdateAllow(home, func(file *AllowFile) error {
				file.Rules = append(file.Rules, AllowRule{ID: fmt.Sprintf("r%d", i), Principal: fmt.Sprintf("p%d", i), Actions: []string{"object.read"}})
				return nil
			})
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	file, err := ReadAllow(home)
	if err != nil {
		t.Fatal(err)
	}
	if len(file.Rules) != grants {
		t.Fatalf("concurrent policy updates were lost: %d of %d kept", len(file.Rules), grants)
	}
}

func TestRefusedAllowUpdateWritesNothing(t *testing.T) {
	home := t.TempDir()
	refusal := fmt.Errorf("refused")
	if err := UpdateAllow(home, func(file *AllowFile) error {
		file.Rules = append(file.Rules, AllowRule{ID: "r", Principal: "p"})
		return refusal
	}); err != refusal {
		t.Fatalf("refusal = %v", err)
	}
	file, err := ReadAllow(home)
	if err != nil || len(file.Rules) != 0 {
		t.Fatalf("refused update persisted: %+v %v", file, err)
	}
}
