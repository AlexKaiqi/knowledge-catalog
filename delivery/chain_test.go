package delivery

import (
	"testing"

	"kc/kernel"
	objectmodel "kc/object"
)

func sampleEnvelope(body string) Envelope {
	return Envelope{
		ID: objectmodel.PinnedObjectRef{
			ObjectRef: objectmodel.ObjectRef{Repository: "kr://acme/public/core", Object: "runbook/public"},
			Commit:    "c1",
		},
		Address: objectmodel.Address{ObjectID: "runbook/public"},
		Value:   map[string]any{"body": body},
		Provenance: &objectmodel.ProvenanceEnvelope{
			ProducedAt: "2026-09-03T00:00:00Z",
		},
		Observations: []objectmodel.UnitObservation{{}},
		Units:        []objectmodel.Address{{ObjectID: "runbook/public"}},
		Declarations: []objectmodel.UnitDeclaration{{Address: objectmodel.Address{ObjectID: "runbook/public"}}},
	}
}

func TestEmptyChainReturnsHydratedBody(t *testing.T) {
	env := sampleEnvelope("secret")
	got, err := Chain{}.Apply(Context{Principal: "bot"}, env)
	if err != nil {
		t.Fatal(err)
	}
	if asMap(t, got.Value)["body"] != "secret" {
		t.Fatalf("empty chain must not rewrite Canonical: %#v", got)
	}
}

func TestRepositoryReadStripsUnauthorizedBodyAndKeepsID(t *testing.T) {
	env := sampleEnvelope("secret procedure")
	got, err := Chain{RepositoryRead{Allowed: func(string, objectmodel.ObjectRef) bool { return false }}}.
		Apply(Context{Principal: "bot"}, env)
	if err != nil {
		t.Fatal(err)
	}
	if got.Value != nil || got.Provenance != nil || got.Observations != nil || got.Units != nil || got.Declarations != nil {
		t.Fatalf("unauthorized body escaped the delivery chain: %#v", got)
	}
	if got.ID.Object != "runbook/public" || got.ID.Repository != "kr://acme/public/core" || got.ID.Commit != "c1" {
		t.Fatalf("stripped envelope lost object ID: %#v", got)
	}
	if got.Address.ObjectID != "runbook/public" {
		t.Fatalf("stripped envelope lost Address: %#v", got)
	}
}

func TestRepositoryReadKeepsAuthorizedBody(t *testing.T) {
	env := sampleEnvelope("public procedure")
	got, err := Chain{RepositoryRead{Allowed: func(principal string, ref objectmodel.ObjectRef) bool {
		return principal == "bot" && ref.Repository == "kr://acme/public/core"
	}}}.Apply(Context{Principal: "bot"}, env)
	if err != nil {
		t.Fatal(err)
	}
	if asMap(t, got.Value)["body"] != "public procedure" {
		t.Fatalf("authorized body was stripped: %#v", got)
	}
}

func TestChainRejectsIdentityMutation(t *testing.T) {
	_, err := Chain{StageFunc(func(_ Context, env Envelope) (Envelope, error) {
		env.ID.Object = "runbook/other"
		return env, nil
	})}.Apply(Context{}, sampleEnvelope("x"))
	if kernel.CodeOf(err) != kernel.ErrPreconditionFailed {
		t.Fatalf("identity mutation must fail closed: %v", err)
	}
	_, err = Chain{StageFunc(func(_ Context, env Envelope) (Envelope, error) {
		env.Address.ObjectID = "runbook/other"
		return env, nil
	})}.Apply(Context{}, sampleEnvelope("x"))
	if kernel.CodeOf(err) != kernel.ErrPreconditionFailed {
		t.Fatalf("Address mutation must fail closed: %v", err)
	}
}

func TestLaterStageMayRewriteVisibleBody(t *testing.T) {
	got, err := Chain{
		RepositoryRead{Allowed: func(string, objectmodel.ObjectRef) bool { return true }},
		StageFunc(func(_ Context, env Envelope) (Envelope, error) {
			env.Value = map[string]any{"body": "redacted"}
			return env, nil
		}),
	}.Apply(Context{Principal: "bot"}, sampleEnvelope("secret procedure"))
	if err != nil {
		t.Fatal(err)
	}
	if asMap(t, got.Value)["body"] != "redacted" {
		t.Fatalf("later stage must be able to rewrite visible content: %#v", got)
	}
	if got.ID.Object != "runbook/public" || got.ID.Commit != "c1" {
		t.Fatalf("content rewrite must keep object ID: %#v", got)
	}
}

func TestChainRunsLaterStagesOnStrippedEnvelope(t *testing.T) {
	var saw any
	got, err := Chain{
		RepositoryRead{Allowed: func(string, objectmodel.ObjectRef) bool { return false }},
		StageFunc(func(_ Context, env Envelope) (Envelope, error) {
			saw = env.Value
			return env, nil
		}),
	}.Apply(Context{Principal: "bot"}, sampleEnvelope("secret"))
	if err != nil {
		t.Fatal(err)
	}
	if saw != nil {
		t.Fatalf("later stage must observe the stripped body, got %#v", saw)
	}
	if got.ID.Object != "runbook/public" {
		t.Fatalf("later stage must not drop object ID: %#v", got)
	}
}

func TestFromValueUsesObjectRepositoryWhenObjectRefDisagrees(t *testing.T) {
	value := objectmodel.ObjectValue{
		ObjectRef:  objectmodel.ObjectRef{Repository: "kr://kc/system", Object: "metric/gmv"},
		Repository: "kr://scene/object",
		Address:    objectmodel.Address{ObjectID: "metric/gmv"},
		Value:      map[string]any{"definition": map[string]any{"name": "GMV"}},
	}
	env := FromValue(value, nil)
	if env.ID.Repository != "kr://scene/object" || env.ID.Object != "metric/gmv" {
		t.Fatalf("delivery ID must follow the object's repository: %#v", env.ID)
	}
	got, err := Chain{RepositoryRead{Allowed: func(_ string, ref objectmodel.ObjectRef) bool {
		return ref.Repository == "kr://kc/system"
	}}}.Apply(Context{Principal: "taihu:alice"}, env)
	if err != nil {
		t.Fatal(err)
	}
	if got.Value != nil {
		t.Fatalf("mismatched ObjectRef.Repository must not grant Canonical: %#v", got)
	}
}

func TestFromValueRoundTripWritesOnlyBody(t *testing.T) {
	value := objectmodel.ObjectValue{
		ObjectRef:    objectmodel.ObjectRef{Repository: "kr://acme/public/core", Object: "runbook/public"},
		Repository:   "kr://acme/public/core",
		Commit:       "c1",
		Address:      objectmodel.Address{ObjectID: "runbook/public"},
		Value:        map[string]any{"body": "secret"},
		Provenance:   &objectmodel.ProvenanceEnvelope{ProducedAt: "t"},
		Units:        []objectmodel.Address{{ObjectID: "runbook/public"}},
		Declarations: []objectmodel.UnitDeclaration{{Address: objectmodel.Address{ObjectID: "runbook/public"}}},
	}
	env := FromValue(value, []objectmodel.UnitObservation{{}})
	stripped, err := Chain{RepositoryRead{}}.Apply(Context{Principal: "bot"}, env)
	if err != nil {
		t.Fatal(err)
	}
	out, observations := stripped.WriteBody(value)
	if out.Value != nil || out.Provenance != nil || observations != nil || out.Units != nil || out.Declarations != nil {
		t.Fatalf("WriteBody leaked Canonical: %#v %#v", out, observations)
	}
	if out.ObjectRef.Object != "runbook/public" || out.Commit != "c1" {
		t.Fatalf("WriteBody changed identity: %#v", out)
	}
}

func asMap(t *testing.T, value any) map[string]any {
	t.Helper()
	object, ok := value.(map[string]any)
	if !ok {
		t.Fatalf("want map, got %#v", value)
	}
	return object
}
