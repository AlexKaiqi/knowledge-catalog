package server

import (
	"errors"
	"testing"
)

func TestManagedHTTPStartActivatesSuccessfulRuntimeOnlyOnce(t *testing.T) {
	reads, writes := 0, 0
	handler := &ManagedHTTPHandler{activate: func() error { reads++; writes++; return nil }}
	for i := 0; i < 3; i++ {
		if err := handler.Start(); err != nil {
			t.Fatal(err)
		}
	}
	if reads != 1 || writes != 1 {
		t.Fatalf("repeated Start reentered activation I/O: reads=%d writes=%d", reads, writes)
	}
}

func TestManagedHTTPStartDoesNotRememberFailedActivation(t *testing.T) {
	calls := 0
	failure := errors.New("first activation failed")
	handler := &ManagedHTTPHandler{activate: func() error {
		calls++
		if calls == 1 {
			return failure
		}
		return nil
	}}
	if err := handler.Start(); !errors.Is(err, failure) {
		t.Fatal(err)
	}
	if err := handler.Start(); err != nil {
		t.Fatal(err)
	}
	if err := handler.Start(); err != nil || calls != 2 {
		t.Fatalf("activation success/failure boundary was lost: calls=%d error=%v", calls, err)
	}
}
