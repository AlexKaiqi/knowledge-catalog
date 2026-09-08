package home

import "testing"

func TestAdmissionPolicyRejectsImplicitOrBroadAuthority(t *testing.T) {
	base := DeploymentConfig{Catalogs: []CatalogBinding{{ID: "kr://platform/catalog"}}}
	for _, policy := range []*AdmissionConfig{
		{Enabled: true, Catalog: "kr://platform/catalog", Actions: []string{"catalog.read"}},
		{Enabled: true, Catalog: "kr://platform/catalog", AuthenticatedUsers: true, Principals: []string{"kaiqidong"}, Actions: []string{"catalog.read"}},
		{Enabled: true, Catalog: "kr://other/catalog", AuthenticatedUsers: true, Actions: []string{"catalog.read"}},
		{Enabled: true, Catalog: "kr://platform/catalog", AuthenticatedUsers: true, Actions: []string{"*"}},
		{Enabled: true, Catalog: "kr://platform/catalog", AuthenticatedUsers: true, Actions: []string{"knowledge.read"}},
		{Enabled: true, Catalog: "kr://platform/catalog", Principals: []string{"service:admin"}, Actions: []string{"catalog.read"}},
	} {
		base.Admission = policy
		if err := validateAdmissionConfig(base); err == nil {
			t.Fatalf("accepted overbroad/ambiguous admission policy: %#v", policy)
		}
	}
	base.Admission = &AdmissionConfig{Enabled: true, Catalog: "kr://platform/catalog", AuthenticatedUsers: true, Actions: []string{"catalog.read", "catalog.repositories.create", "catalog.repositories.connect"}}
	if err := validateAdmissionConfig(base); err != nil {
		t.Fatal(err)
	}
}
