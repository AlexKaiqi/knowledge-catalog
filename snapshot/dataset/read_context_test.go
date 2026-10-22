package dataset_test

import (
	"context"
	"errors"
	"kc/internal/testkit"
	"kc/kernel"
	"kc/snapshot"
	catalog "kc/snapshot/dataset"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestDatasetReadContextCancelsAuthorityIOWithoutChangingRoot(t *testing.T) {
	for _, registryRead := range []bool{false, true} {
		name := "source"
		if registryRead {
			name = "registry"
		}
		t.Run(name, func(t *testing.T) {
			var block atomic.Bool
			var entered atomic.Bool
			target := ""
			fake := testkit.NewLakeFSFakeWithInterceptor(t, func(w http.ResponseWriter, r *http.Request) bool {
				if block.Load() && r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/repositories/"+target+"/") {
					entered.Store(true)
					<-r.Context().Done()
					return true
				}
				return false
			})
			sourceName, registryName := fake.NewRepo(), fake.NewRepo()
			source, err := snapshot.OpenLakeFS("kr://context/source", fake.DSN(sourceName), fake.Credential())
			if err != nil {
				t.Fatal(err)
			}
			authority, err := snapshot.OpenLakeFS("kr://context/registry", fake.DSN(registryName), fake.Credential())
			if err != nil {
				t.Fatal(err)
			}
			registered, err := catalog.CreateSnapshotRegistry(authority, snapshot.DefaultRef)
			if err != nil {
				t.Fatal(err)
			}
			stores := snapshot.NewRegistry()
			if err := stores.Add(source); err != nil {
				t.Fatal(err)
			}
			original, err := catalog.NewService(stores, registered)
			if err != nil {
				t.Fatal(err)
			}
			if err := original.RegisterRepository(source.ID()); err != nil {
				t.Fatal(err)
			}
			sources := []catalog.DatasetSource{{Repository: source.ID(), Selector: snapshot.DefaultRef, Path: catalog.MountPath("source")}}
			definition, err := original.DefineDataset("fixed", 1, sources)
			if err != nil {
				t.Fatal(err)
			}
			pin, err := original.ResolveDataset("fixed")
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 80*time.Millisecond)
			defer cancel()
			view, err := original.(catalog.ReadContextBound).ReadContext(ctx)
			if err != nil {
				t.Fatal(err)
			}
			target = sourceName
			if registryRead {
				target = registryName
			}
			block.Store(true)
			started := time.Now()
			if registryRead {
				_, err = view.Log(catalog.LogQuery{Limit: 1})
			} else {
				_, err = view.ResolveDefinition(definition)
			}
			if !errors.Is(err, context.DeadlineExceeded) || ctx.Err() != context.DeadlineExceeded || !entered.Load() || time.Since(started) > time.Second {
				t.Fatalf("request context did not cancel actual %s I/O: %v context=%v entered=%v elapsed=%s", name, err, ctx.Err(), entered.Load(), time.Since(started))
			}
			block.Store(false)
			if _, err := view.Log(catalog.LogQuery{Limit: 1}); !errors.Is(err, context.DeadlineExceeded) {
				t.Fatalf("canceled registry query succeeded or changed failure: %v", err)
			}
			if _, err := view.ReadVirtualFileAt(definition, pin, "source/absent.txt"); !errors.Is(err, context.DeadlineExceeded) {
				t.Fatalf("canceled virtual read consulted source availability: %v", err)
			}

			if _, err := view.ReplayPublished(pin); !errors.Is(err, context.DeadlineExceeded) {
				t.Fatalf("cached source availability converted canceled replay to success: %v", err)
			}
			if check := view.CheckResolved(pin); len(check.Issues) == 0 {
				t.Fatal("cached source availability hid canceled validation")
			}
			if _, err := original.ResolveDefinition(definition); err != nil {
				t.Fatalf("request canceled root authority: %v", err)
			}
			if err := view.RegisterRepository("kr://context/forbidden"); kernel.CodeOf(err) != kernel.ErrPreconditionFailed {
				t.Fatalf("context view became writable: %v", err)
			}
			if _, err := original.DefineDataset("later", 1, sources); err != nil {
				t.Fatal(err)
			}
			if _, err := view.Set("later"); kernel.CodeOf(err) != kernel.ErrDatasetInvalid {
				t.Fatalf("context view followed a later registry version: %v", err)
			}
		})
	}
}
