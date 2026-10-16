package liveaccess_test

import (
	"testing"

	"kc/internal/bindingfixture"
	"kc/kernel"
	"kc/liveaccess"
	objectmodel "kc/object"
)

// The resolved declaration carries the descriptor's identity promise so the
// index side can refuse per-caller inputs without re-reading the descriptor;
// an undeclared identity resolves as per-caller, never as shared.
func TestResolvedDeclarationCarriesDescriptorIdentity(t *testing.T) {
	address := objectmodel.Address{Kind: objectmodel.KindAspect, ObjectID: "table/orders", AspectName: "stats"}
	source := &objectmodel.ValueSource{Kind: objectmodel.ValueSourceBinding, Binding: &objectmodel.BindingDeclaration{DescriptorRef: "resource/stats"}}
	for declared, want := range map[string]objectmodel.ResultIdentity{
		"":           objectmodel.IdentityPerCaller,
		"per-caller": objectmodel.IdentityPerCaller,
		"shared":     objectmodel.IdentityShared,
	} {
		body := bindingfixture.Descriptor("https://stats.example")
		if declared != "" {
			body["identity"] = declared
		}
		scope := func(objectmodel.ObjectID) (liveaccess.DescriptorAt, error) {
			return liveaccess.DescriptorAt{Repository: "kr://ops/shared", Commit: "c1", Value: body}, nil
		}
		got, err := liveaccess.ResolveDeclaration(scope, kernel.RepositoryID("kr://tables/meta"), "c2", objectmodel.UnitDeclaration{Address: address, ValueSource: source})
		if err != nil || got.Identity != want {
			t.Fatalf("declared %q: got %q %v", declared, got.Identity, err)
		}
	}
}
