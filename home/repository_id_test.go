package home

import (
	"strings"
	"testing"
)

func TestNormalizeRepositoryIDAcceptsGravelerName(t *testing.T) {
	id, err := NormalizeRepositoryID("table-meta")
	if err != nil || id != "table-meta" {
		t.Fatalf("Graveler name: %q %v", id, err)
	}
	id, err = NormalizeRepositoryID("kr://acme/table-meta")
	if err != nil || id != "kr://acme/table-meta" {
		t.Fatalf("kr id: %q %v", id, err)
	}
	if _, err := NormalizeRepositoryID("kc-system"); err == nil || !strings.Contains(err.Error(), "Graveler") {
		t.Fatalf("platform Graveler name: %v", err)
	}
	if _, err := NormalizeRepositoryID("物理层"); err == nil {
		t.Fatal("non-slug name")
	}
}
