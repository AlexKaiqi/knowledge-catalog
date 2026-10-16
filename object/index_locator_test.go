package object_test

import (
	"testing"

	objectmodel "kc/object"
)

func TestIndexFieldLocatorUsesObjectPointerEncoding(t *testing.T) {
	for _, tc := range []struct{ fragment, field string }{
		{"/fields/%E7%BB%9F%E8%AE%A1%20%E5%90%8D%E7%A7%B0", "统计 名称"},
		{"/fields/a~1b", "a/b"},
		{"/fields/t~0n", "t~n"},
		{"/fields/rate%2525", "rate%25"},
		{"/fields/a~01b", "a~1b"},
	} {
		t.Run(tc.field, func(t *testing.T) {
			locator, err := objectmodel.ParseFieldLocator("schema/record@fixed#" + tc.fragment)
			if err != nil || locator.Path != tc.field || locator.Schema.Commit != "fixed" {
				t.Fatalf("valid Schema field pointer: %#v %v", locator, err)
			}
			again, err := objectmodel.ParseFieldLocator(locator.String())
			if err != nil || again != locator {
				t.Fatalf("locator changed on roundtrip: %#v -> %s -> %#v (%v)", locator, locator.String(), again, err)
			}
		})
	}
}

func TestIndexFieldLocatorRejectsWildcardsAndInvalidPointers(t *testing.T) {
	for _, fragment := range []string{"/fields/*", "/fields/%2A", "/fields/a?", "/fields/a/b", "/fields/a~2b", "/fields/%zz", "/fields/", "/fields/a#b", "/other/a"} {
		if _, err := objectmodel.ParseFieldLocator("schema/record#" + fragment); err == nil {
			t.Errorf("accepted non-field or wildcard pointer %q", fragment)
		}
	}
}
