package cli

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"kc/kernel"
	"kc/object/client"
)

func TestBasicObjectReferenceFlagAliasCannotReachTransport(t *testing.T) {
	for _, path := range []string{"read", "writer put", "writer remove"} {
		for _, alias := range []struct {
			name  string
			value any
		}{
			{"single", testReference}, {"repeated", []string{testReference, testReference}}, {"missing-value", true},
		} {
			t.Run(path+"/"+alias.name, func(t *testing.T) {
				remote := &fakeClient{}
				flags := map[string]any{"reference": alias.value}
				if strings.HasPrefix(path, "writer ") {
					flags["command-id"] = "must-not-write"
					if path == "writer put" {
						flags["value"] = "null"
					}
				}
				_, err := New(remote, Context{}).Execute(context.Background(), path, flags, client.RequestOptions{})
				if kernel.CodeOf(err) != kernel.ErrUsageInvalid || len(remote.calls) != 0 || !strings.Contains(err.Error(), "object URN") {
					t.Fatalf("%s --reference reached transport or recommended an alias: error=%v calls=%v", path, err, remote.calls)
				}
			})
		}
	}
}

func TestBasicObjectOperandBindingRejectsDuplicateTarget(t *testing.T) {
	for _, path := range []string{"read", "writer put", "writer remove"} {
		for _, alias := range []any{testReference, "urn:kc:object:one", "", true, []string{testReference, testReference}} {
			remote := &fakeClient{}
			flags := map[string]any{"reference": alias}
			err := BindOperands(path, flags, []string{testReference})
			if kernel.CodeOf(err) != kernel.ErrUsageInvalid || len(remote.calls) != 0 || !strings.Contains(err.Error(), "object URN") {
				t.Fatalf("%s accepted two target forms: %v calls=%v", path, err, remote.calls)
			}
			if !reflect.DeepEqual(flags["reference"], alias) {
				t.Fatalf("failed binding replaced the caller's target: %#v", flags)
			}
		}
	}
}

func TestBasicObjectOperandBindingRejectsExtraOrMalformedTargets(t *testing.T) {
	for _, path := range []string{"read", "writer put", "writer remove"} {
		flags := map[string]any{}
		if err := BindOperands(path, flags, []string{testReference, testReference}); kernel.CodeOf(err) != kernel.ErrUsageInvalid || len(flags) != 0 {
			t.Fatalf("extra positional changed the command: %v %#v", err, flags)
		}
		for _, raw := range []string{"table/orders", "#/columns/id", ""} {
			remote := &fakeClient{}
			flags := map[string]any{}
			if strings.HasPrefix(path, "writer ") {
				flags["command-id"] = "must-not-write"
				if path == "writer put" {
					flags["value"] = "null"
				}
			}
			if err := BindOperands(path, flags, []string{raw}); err != nil {
				t.Fatal(err)
			}
			_, err := New(remote, Context{}).Execute(context.Background(), path, flags, client.RequestOptions{})
			if kernel.CodeOf(err) != kernel.ErrUsageInvalid || len(remote.calls) != 0 {
				t.Fatalf("malformed positional reached transport: %v calls=%v", err, remote.calls)
			}
		}
	}
}
