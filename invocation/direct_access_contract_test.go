package invocation

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"reflect"
	"strings"
	"testing"
	"time"

	"kc/home"
	"kc/internal/testkit"
	"kc/invoke"
	"kc/kernel"
	"kc/liveaccess"
	objectmodel "kc/object"
	"kc/observability"
	"kc/snapshot"
)

type directAccessLookup struct {
	requests []liveaccess.StateLookupRequest
	result   liveaccess.StateObservation
	err      error
	source   liveaccess.StateLookup
}

func (s *directAccessLookup) LookupState(ctx context.Context, request liveaccess.StateLookupRequest) (liveaccess.StateObservation, error) {
	s.requests = append(s.requests, request)
	if s.source != nil {
		return s.source.LookupState(ctx, request)
	}
	return s.result, s.err
}

type directAccessRoundTrip func(*http.Request) (*http.Response, error)

func (f directAccessRoundTrip) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

func directAccessProof() http.Header {
	return http.Header{
		"Authorization":  []string{"Bearer direct-access-secret"},
		"X-Tai-Identity": []string{"direct-access-identity-proof"},
	}
}

func directAccessHTTPSource(t *testing.T, lookup *directAccessLookup) liveaccess.StateLookup {
	t.Helper()
	client := &http.Client{Transport: directAccessRoundTrip(func(request *http.Request) (*http.Response, error) {
		if request.Header.Get("X-Resource-Principal") != "consumer" || request.Header.Get("X-Resource-Request-Id") != "direct-access-contract" {
			t.Fatal("source lost the verified caller or request correlation")
		}
		for name, values := range directAccessProof() {
			if request.Header.Get(name) != values[0] {
				t.Fatalf("source did not receive the caller's %s proof", name)
			}
		}
		payload, err := io.ReadAll(request.Body)
		if err != nil {
			t.Fatal(err)
		}
		assertDirectAccessNoProof(t, payload)
		status, response := http.StatusOK, any(lookup.result)
		if lookup.err != nil {
			status = http.StatusServiceUnavailable
			response = map[string]any{"error": kernel.Normalize(lookup.err)}
		}
		raw, err := json.Marshal(response)
		if err != nil {
			t.Fatal(err)
		}
		return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(string(raw)))}, nil
	})}
	adapter, err := liveaccess.NewHTTPStateLookupWithOptions(liveaccess.HTTPStateLookupOptions{Client: client, CredentialOrigins: []string{"https://runtime.example"}})
	if err != nil {
		t.Fatal(err)
	}
	return adapter
}

func assertDirectAccessNoProof(t *testing.T, raw []byte) {
	t.Helper()
	for name, values := range directAccessProof() {
		if strings.Contains(string(raw), values[0]) || strings.Contains(string(raw), strings.TrimPrefix(values[0], "Bearer ")) {
			t.Fatalf("%s authentication proof escaped into a request body or audit record", name)
		}
	}
}

func directAccessFixture(t *testing.T) (*home.Home, map[string]invoke.FlagValue, liveaccess.ResolvedBinding) {
	t.Helper()
	s := testkit.NewSetup(t, "kr://regression/direct-access")
	address := objectmodel.Address{Kind: objectmodel.KindAspect, ObjectID: "Service:orders", AspectName: "health"}
	receipt, err := s.Writer.Commit("direct-access-declaration", objectmodel.CommitChangeSet{
		TargetRepository: s.RepositoryID, TargetRef: snapshot.DefaultRef,
		BaseCommit: s.RootCommitID, ExpectedTargetCommit: s.RootCommitID,
		Operations: []objectmodel.Operation{
			{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "schema/service.health"}, Value: map[string]any{
				"entity": "Service", "aspect": "health", "origin": "https://runtime.example",
				"fields": map[string]any{"status": map[string]any{"type": "string", "required": true}},
			}},
			{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindAspect, ObjectID: address.ObjectID, AspectName: "properties"}, Value: map[string]any{"name": "orders"}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	commit := receipt.Result.CommitID
	binding, err := liveaccess.ResolveRepoBinding(s.Repo, commit, address)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if err := WriteAllow(dir, AllowFile{Rules: []AllowRule{{
		ID: "direct-access-reader", Principal: "consumer", Actions: []string{"resource.access"},
		Repo: string(s.RepositoryID), Object: string(address.ObjectID), Aspect: address.AspectName,
	}}}); err != nil {
		t.Fatal(err)
	}
	flags := map[string]invoke.FlagValue{
		"home": dir, "as": "consumer", "repo": string(s.RepositoryID), "commit": string(commit),
		"object": string(address.ObjectID), "aspect": address.AspectName, "request-id": "direct-access-contract",
	}
	return &home.Home{Dir: dir, Store: s.Store, Reader: s.Reader}, flags, binding
}

func directAccessObservationBasis() objectmodel.ObservationBasis {
	return objectmodel.ObservationBasis{
		BindingGeneration: "health-generation-1", Consistency: objectmodel.ObservationBounded,
		SourceRevision: "health-7", ObservedAt: "2026-10-13T04:00:00Z",
	}
}

func assertDirectAccessLookup(t *testing.T, lookup *directAccessLookup, binding liveaccess.ResolvedBinding) {
	t.Helper()
	if len(lookup.requests) != 1 {
		t.Fatalf("access did not reach exactly one source lookup: %d", len(lookup.requests))
	}
	request := lookup.requests[0]
	if !reflect.DeepEqual(request.Binding, binding) || request.SchemaRef != binding.SchemaRef || request.Origin != binding.Origin {
		t.Fatalf("access changed its fixed Binding or adopted Schema: %#v", request)
	}
	if request.Identity.Principal != "consumer" || request.RequestID != "direct-access-contract" {
		t.Fatalf("access lost the explicit caller or request: %#v", request)
	}
}

// resource-access U2 requires direct access to reject unobserved placeholders
// and values outside the adopted Schema, just as logical State READ does.
func TestDirectResourceAccessValidatesPinnedObservation(t *testing.T) {
	valid := directAccessObservationBasis()
	missingTime := valid
	missingTime.ObservedAt = ""
	for _, test := range []struct {
		name  string
		value any
		basis objectmodel.ObservationBasis
		code  kernel.ErrorCode
	}{
		{"missing-basis", map[string]any{"status": "healthy"}, objectmodel.ObservationBasis{}, kernel.ErrCapabilityUnsatisfied},
		{"missing-observation-time", map[string]any{"status": "healthy"}, missingTime, kernel.ErrCapabilityUnsatisfied},
		{"placeholder-null", nil, objectmodel.ObservationBasis{}, kernel.ErrCapabilityUnsatisfied},
		{"wrong-adopted-field-type", map[string]any{"status": 42}, valid, kernel.ErrSchemaInstanceInvalid},
		{"valid-value", map[string]any{"status": "healthy"}, valid, ""},
		{"observed-null", nil, valid, ""},
	} {
		t.Run(test.name, func(t *testing.T) {
			ws, flags, binding := directAccessFixture(t)
			lookup := &directAccessLookup{result: liveaccess.StateObservation{Value: test.value, Basis: test.basis}}
			output, err := ExecuteApplicationOperation(context.Background(), "object-access", "resource.access",
				Command{Stage: invoke.StageGoverned, Run: VerbResourceAccess}, flags, lookup, ws, ws.Dir, nil)
			assertDirectAccessLookup(t, lookup, binding)
			if test.code != "" {
				if kernel.CodeOf(err) != test.code || accessOutput(output) != nil {
					t.Fatalf("invalid observation escaped direct access: output=%#v error=%v; want %s", output, err, test.code)
				}
				return
			}
			if err != nil {
				t.Fatalf("valid observation was rejected: %v", err)
			}
			envelope, ok := accessOutput(output).(map[string]any)
			if !ok || !reflect.DeepEqual(envelope["bindings"], []liveaccess.ResolvedBinding{binding}) ||
				!reflect.DeepEqual(envelope["observations"], []liveaccess.StateObservation{lookup.result}) {
				t.Fatalf("access changed its public envelope or successful observation (including null): %#v", output)
			}
		})
	}
}

func TestDirectResourceAccessRecordsActualAccessEvidence(t *testing.T) {
	for _, test := range []struct {
		name, outcome string
		sourceErr     error
	}{
		{"success", "RESOLVED", nil},
		{"source-failure", "ERROR", kernel.Fail(kernel.ErrTemporaryUnavailable, "source Authorization / X-Tai-Identity verifier unavailable")},
	} {
		t.Run(test.name, func(t *testing.T) {
			ws, flags, binding := directAccessFixture(t)
			lookup := &directAccessLookup{
				result: liveaccess.StateObservation{Value: map[string]any{"status": "healthy"}, Basis: directAccessObservationBasis()},
				err:    test.sourceErr,
			}
			lookup.source = directAccessHTTPSource(t, lookup)
			ctx := liveaccess.ContextWithCallerCredentials(context.Background(), liveaccess.CallerCredentialsFromHeader(directAccessProof()))
			started := time.Now().UTC()
			result := InvokeApplicationWithTelemetryAtHome(ctx, nil, "object-access", "resource.access",
				Command{Stage: invoke.StageGoverned, Run: VerbResourceAccess}, flags, lookup, ws)
			finished := time.Now().UTC()
			assertDirectAccessLookup(t, lookup, binding)
			if (result.Status == 0) != (test.sourceErr == nil) {
				t.Fatalf("source outcome was changed: %#v", result)
			}
			assertDirectAccessNoProof(t, []byte(result.Stdout))
			var response struct{ Error *kernel.IngressError }
			if test.sourceErr != nil {
				if err := json.Unmarshal([]byte(result.Stdout), &response); err != nil || response.Error == nil ||
					response.Error.Code != kernel.CodeOf(test.sourceErr) || !strings.Contains(response.Error.Message, kernel.Normalize(test.sourceErr).Message) {
					t.Fatalf("source failure lost its public error classification: output=%s decode=%v", result.Stdout, err)
				}
			}
			page, err := observability.NewFileStore(ws.Dir).Access(context.Background(), observability.AccessQuery{
				Principal: "consumer", Action: "resource.access", Repository: binding.Repository, Object: binding.Address.ObjectID,
			})
			if err != nil || len(page.Entries) != 1 {
				t.Fatalf("public access emitted no actual access evidence: entries=%#v error=%v", page.Entries, err)
			}
			event := page.Entries[0]
			if event.Result != test.outcome || event.Identity.Principal != "consumer" || event.RequestID != "direct-access-contract" {
				t.Fatalf("access evidence lost caller, request, or source outcome: %#v", event)
			}
			occurred, err := time.Parse(time.RFC3339Nano, event.OccurredAt)
			if err != nil || occurred.Before(started) || occurred.After(finished) {
				t.Fatalf("access evidence time is outside the actual invocation: %q", event.OccurredAt)
			}
			if test.sourceErr != nil && (event.Error["code"] != string(kernel.CodeOf(test.sourceErr)) || event.Error["message"] != response.Error.Message) {
				t.Fatalf("failed access evidence lost its source error: %#v", event)
			}
			trail, err := ReadTrail(ws.Dir, "", "", 1)
			if err != nil || len(trail) != 1 || trail[0].EvidenceID != event.EvidenceID {
				t.Fatalf("audit trail did not identify this access evidence: %#v %v", trail, err)
			}
			audit, err := json.Marshal([]any{event, trail})
			if err != nil {
				t.Fatal(err)
			}
			assertDirectAccessNoProof(t, audit)
			for _, target := range event.Object {
				if target.ObjectRef.Repository != binding.Repository || target.ObjectRef.Object != binding.Address.ObjectID {
					continue
				}
				if target.ObjectRef.Commit != binding.DeclarationCommit || target.Address == nil || *target.Address != binding.Address {
					t.Fatalf("access evidence lost its fixed declaration: %#v", target)
				}
				if test.sourceErr == nil && (len(target.Observations) != 1 || target.Observations[0].DeclarationDigest != binding.DeclarationDigest || target.Observations[0].Basis != lookup.result.Basis) {
					t.Fatalf("successful access evidence lost its actual observation: %#v", target)
				}
				if test.sourceErr != nil && len(target.Observations) != 0 {
					t.Fatalf("failed access evidence invented a successful observation: %#v", target)
				}
				return
			}
			t.Fatalf("access evidence omitted the actual Binding target: %#v", event)
		})
	}
}
