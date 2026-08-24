package main

import "testing"

func TestTranslateStableIdentityAndDigest(t *testing.T) {
	source := sourceFile{SourceRef: "file://facts", CapturedAt: "2026-08-24T00:00:00Z", Facts: []fact{{Key: "b", Value: "two"}, {Key: "a", Value: "one"}}}
	units, observed, err := translate(source)
	if err != nil {
		t.Fatal(err)
	}
	if len(units) != 2 || len(observed) != 2 {
		t.Fatalf("unexpected output sizes: %d %d", len(units), len(observed))
	}
	if got := string(units[0].Address.ObjectID); got != "FileFact:a" {
		t.Fatalf("stable sorted identity: %s", got)
	}
}
