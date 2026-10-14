package service

import (
	"reflect"
	"testing"

	"kc/kernel"
	objectmodel "kc/object"
)

func TestBasicReadTasksExposeOnlyReferenceCoordinates(t *testing.T) {
	for _, contract := range []struct {
		name   string
		shape  reflect.Type
		fields map[string]reflect.Type
	}{
		{"repository", reflect.TypeOf(ReadRequest{}), map[string]reflect.Type{"Reference": reflect.TypeOf(objectmodel.Reference{}), "Commit": reflect.TypeOf(kernel.CommitID(""))}},
		{"dataset", reflect.TypeOf(DatasetReadRequest{}), map[string]reflect.Type{"Reference": reflect.TypeOf(objectmodel.Reference{})}},
	} {
		t.Run(contract.name, func(t *testing.T) {
			if contract.shape.NumField() != len(contract.fields) {
				t.Fatalf("basic task still exposes alternate content coordinates: %v", contract.shape)
			}
			for name, expected := range contract.fields {
				field, present := contract.shape.FieldByName(name)
				if !present || field.Type != expected {
					t.Fatalf("%s does not require the owned reference/basis: %v", name, field.Type)
				}
			}
		})
	}
	port := reflect.TypeOf((*CanonicalReader)(nil)).Elem()
	if port.NumMethod() != 1 || port.Method(0).Name != "ReadReference" {
		t.Fatalf("basic read task still requires separate-address ports: %v", port)
	}
}
