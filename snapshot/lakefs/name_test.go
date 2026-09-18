package lakefs_test

import (
	"strings"
	"testing"

	"kc/kernel"
	"kc/snapshot/lakefs"
)

func TestManagedGravelerNameUsesSlug(t *testing.T) {
	got, err := lakefs.ManagedGravelerName("table-meta", "admin", "4efb667886bab5ab6c8c6af59245edac")
	if err != nil || got != "table-meta" {
		t.Fatalf("got %q err=%v", got, err)
	}
}

func TestManagedGravelerNameRejectsPlatformPrefix(t *testing.T) {
	if _, err := lakefs.ManagedGravelerName("kc-system", "admin", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"); err == nil || kernel.CodeOf(err) != kernel.ErrUsageInvalid {
		t.Fatalf("platform prefix: %v", err)
	}
}

func TestManagedGravelerNameRejectsNonSlugName(t *testing.T) {
	if _, err := lakefs.ManagedGravelerName("物理层", "admin", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"); err == nil || kernel.CodeOf(err) != kernel.ErrUsageInvalid || !strings.Contains(err.Error(), "物理层") {
		t.Fatalf("non-slug name: %v", err)
	}
}

func TestManagedGravelerNameUnnamedDoesNotUseOwnerOrKcPrefix(t *testing.T) {
	got, err := lakefs.ManagedGravelerName("", "admin", "4efb667886bab5ab6c8c6af59245edac")
	if err != nil || got != "repo-4efb667886ba" {
		t.Fatalf("unnamed: %q err=%v", got, err)
	}
	if strings.Contains(got, "admin") || strings.HasPrefix(got, "kc-") {
		t.Fatal("unnamed Graveler name included owner or platform prefix")
	}
}
