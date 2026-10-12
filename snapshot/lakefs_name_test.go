package snapshot_test

import (
	"kc/snapshot"
	"strings"
	"testing"

	"kc/kernel"
)

func TestManagedGravelerNameUsesSlug(t *testing.T) {
	got, err := snapshot.ManagedLakeFSName("table-meta", "admin", "4efb667886bab5ab6c8c6af59245edac")
	if err != nil || got != "table-meta" {
		t.Fatalf("got %q err=%v", got, err)
	}
}

func TestManagedGravelerNameRejectsPlatformPrefix(t *testing.T) {
	if _, err := snapshot.ManagedLakeFSName("kc-system", "admin", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"); err == nil || kernel.CodeOf(err) != kernel.ErrUsageInvalid {
		t.Fatalf("platform prefix: %v", err)
	}
}

func TestManagedGravelerNameRejectsNonSlugName(t *testing.T) {
	if _, err := snapshot.ManagedLakeFSName("物理层", "admin", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"); err == nil || kernel.CodeOf(err) != kernel.ErrUsageInvalid || !strings.Contains(err.Error(), "物理层") {
		t.Fatalf("non-slug name: %v", err)
	}
}

func TestManagedGravelerNameUnnamedDoesNotUseOwnerOrKcPrefix(t *testing.T) {
	got, err := snapshot.ManagedLakeFSName("", "admin", "4efb667886bab5ab6c8c6af59245edac")
	if err != nil || got != "repo-4efb667886ba" {
		t.Fatalf("unnamed: %q err=%v", got, err)
	}
	if strings.Contains(got, "admin") || strings.HasPrefix(got, "kc-") {
		t.Fatal("unnamed Graveler name included owner or platform prefix")
	}
}
