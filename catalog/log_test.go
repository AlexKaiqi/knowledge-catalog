package catalog_test

import (
	"testing"

	"kc/catalog"
)

func TestCatalogLogStampsAuthor(t *testing.T) {
	s := setupFed(t)
	s.catalog.SetStamp("agent:payments", "run-42", "rule-9")
	if _, err := s.catalog.DefineView("duty", 1, []catalog.ViewSource{
		{Repository: "kr://acme/public/core", Selector: "refs/heads/main"},
	}); err != nil {
		t.Fatal(err)
	}
	hist := s.catalog.Log(catalog.CatalogLogQuery{Limit: 20, View: "duty"})
	if len(hist.Commits) == 0 {
		t.Fatal(hist)
	}
	var saw bool
	for _, c := range hist.Commits {
		if c.Message != "define-view duty" {
			continue
		}
		saw = true
		if c.Author != "agent:payments" || c.RequestID != "run-42" || c.RuleID != "rule-9" {
			t.Fatalf("%#v", c)
		}
	}
	if !saw {
		t.Fatal(hist.Commits)
	}
}
