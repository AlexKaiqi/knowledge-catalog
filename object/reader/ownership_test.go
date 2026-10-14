package reader_test

import (
	"reflect"
	"testing"

	"kc/object/reader"
)

func TestFixedReaderDoesNotExposeLiveAccessDeclarationInterpretation(t *testing.T) {
	for _, typ := range []reflect.Type{reflect.TypeOf((*reader.Reader)(nil)), reflect.TypeOf((*reader.Serving)(nil))} {
		for _, name := range []string{"ResolveBinding", "ResolveBindingAt"} {
			if _, present := typ.MethodByName(name); present {
				t.Errorf("%s still owns LiveAccess-specific %s", typ, name)
			}
		}
	}
}
