package delivery

import (
	"testing"

	"kc/kernel"
	"kc/knowledge"
)

func sampleEnvelope(body string) Envelope {
	return Envelope{
		ID: knowledge.PinnedKnowledgeRef{
			KnowledgeRef: knowledge.KnowledgeRef{Repository: "kr://acme/public/core", Object: "runbook/public"},
			Commit:       "c1",
		},
		Address: knowledge.Address{ObjectID: "runbook/public"},
		Value:   map[string]any{"body": body},
		Provenance: &knowledge.ProvenanceEnvelope{
			ProducedAt: "2026-09-03T00:00:00Z",
		},
		Observations: []knowledge.UnitObservation{{}},
		Units:        []knowledge.Address{{ObjectID: "runbook/public"}},
		Declarations: []knowledge.UnitDeclaration{{Address: knowledge.Address{ObjectID: "runbook/public"}}},
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
	got, err := Chain{RepositoryRead{Allowed: func(string, knowledge.KnowledgeRef) bool { return false }}}.
		Apply(Context{Principal: "bot"}, env)
	if err != nil {
		t.Fatal(err)
	}
	if got.Value != nil || got.Provenance != nil || got.Observations != nil || got.Units != nil || got.Declarations != nil {
		t.Fatalf("unauthorized body escaped the delivery chain: %#v", got)
	}
	if got.ID.Object != "runbook/public" || got.ID.Repository != "kr://acme/public/core" || got.ID.Commit != "c1" {
		t.Fatalf("stripped envelope lost knowledge ID: %#v", got)
	}
	if got.Address.ObjectID != "runbook/public" {
		t.Fatalf("stripped envelope lost Address: %#v", got)
	}
}

func TestRepositoryReadKeepsAuthorizedBody(t *testing.T) {
	env := sampleEnvelope("public procedure")
	got, err := Chain{RepositoryRead{Allowed: func(principal string, ref knowledge.KnowledgeRef) bool {
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
		RepositoryRead{Allowed: func(string, knowledge.KnowledgeRef) bool { return true }},
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
		t.Fatalf("content rewrite must keep knowledge ID: %#v", got)
	}
}

func TestChainRunsLaterStagesOnStrippedEnvelope(t *testing.T) {
	var saw any
	got, err := Chain{
		RepositoryRead{Allowed: func(string, knowledge.KnowledgeRef) bool { return false }},
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
		t.Fatalf("later stage must not drop knowledge ID: %#v", got)
	}
}

func TestFromValueRoundTripWritesOnlyBody(t *testing.T) {
	value := knowledge.KnowledgeValue{
		KnowledgeRef: knowledge.KnowledgeRef{Repository: "kr://acme/public/core", Object: "runbook/public"},
		Repository:   "kr://acme/public/core",
		Commit:       "c1",
		Address:      knowledge.Address{ObjectID: "runbook/public"},
		Value:        map[string]any{"body": "secret"},
		Provenance:   &knowledge.ProvenanceEnvelope{ProducedAt: "t"},
		Units:        []knowledge.Address{{ObjectID: "runbook/public"}},
		Declarations: []knowledge.UnitDeclaration{{Address: knowledge.Address{ObjectID: "runbook/public"}}},
	}
	env := FromValue(value, []knowledge.UnitObservation{{}})
	stripped, err := Chain{RepositoryRead{}}.Apply(Context{Principal: "bot"}, env)
	if err != nil {
		t.Fatal(err)
	}
	out, observations := stripped.WriteBody(value)
	if out.Value != nil || out.Provenance != nil || observations != nil || out.Units != nil || out.Declarations != nil {
		t.Fatalf("WriteBody leaked Canonical: %#v %#v", out, observations)
	}
	if out.KnowledgeRef.Object != "runbook/public" || out.Commit != "c1" {
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
