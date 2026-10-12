package server

import (
	"net/http"
	"reflect"
	"testing"

	inv "kc/invocation"
)

func TestGrantHTTPUpdatePreservesTheApplicationLedger(t *testing.T) {
	dir, handler := grantTestFacade(t, []inv.AllowRule{
		{ID: "manager", Principal: "operator", Actions: []string{"admin.grants.manage"}},
		{ID: "target", Principal: "reader", Actions: []string{"file.read"}, SharedBy: "publisher"},
	})
	before, err := inv.ReadAllow(dir)
	if err != nil {
		t.Fatal(err)
	}
	body := `{"principal":"reader","actions":["file.read"],"dataset":"narrowed","expected":{"id":"target","principal":"reader","actions":["file.read"],"sharedBy":"publisher"}}`
	updated := grantRequestForTest(t, handler, "operator", http.MethodPut, "/admin/v1/grants/target", body, http.StatusOK)
	if updated["id"] != "target" || updated["sharedBy"] != "publisher" || updated["dataset"] != "narrowed" {
		t.Fatalf("unexpected replacement receipt: %#v", updated)
	}
	grantRequestForTest(t, handler, "operator", http.MethodPut, "/admin/v1/grants/target", body, http.StatusConflict)
	after, err := inv.ReadAllow(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(after.InitialGrants, before.InitialGrants) || !reflect.DeepEqual(after.Admissions, before.Admissions) || len(after.Rules) != 2 || after.Rules[1].Dataset != "narrowed" {
		t.Fatalf("rule update damaged existing application state: %#v", after)
	}
}
