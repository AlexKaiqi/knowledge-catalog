package service

import (
	"context"
	objectmodel "kc/object"
	"testing"

	"kc/kernel"
)

func TestDatasetExecutorsAuthorizeBeforeResolution(t *testing.T) {
	deny := func(context.Context) error { return kernel.Fail(kernel.ErrForbidden, "denied") }
	read := DatasetReadExecutor{Authorize: deny,
		ReferenceRead: func(context.Context, objectmodel.Reference) ([]objectmodel.ObservedValue, error) {
			t.Fatal("denied read resolved data")
			return nil, nil
		},
		Deliver: func(_ context.Context, v []objectmodel.ObservedValue) ([]objectmodel.ObservedValue, error) {
			t.Fatal("denied read delivered data")
			return v, nil
		},
	}
	if _, err := read.Execute(context.Background(), DatasetReadRequest{Reference: objectmodel.Reference{Repository: "kr://app/read", Object: "one"}}); kernel.CodeOf(err) != kernel.ErrForbidden {
		t.Fatal(err)
	}
}
