package opensearchimpl

import (
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"

	ports "kc/discovery/projection"
	"kc/kernel"
)

type rejectProtectionRecord struct{ next http.RoundTripper }

func (r rejectProtectionRecord) RoundTrip(request *http.Request) (*http.Response, error) {
	if request.Method == http.MethodPut && strings.Contains(request.URL.Path, "-retained-") {
		return &http.Response{StatusCode: http.StatusServiceUnavailable, Body: io.NopCloser(strings.NewReader(`{"error":"injected protection write failure"}`)), Header: http.Header{}, Request: request}, nil
	}
	return r.next.RoundTrip(request)
}

func TestLiveOpenSearchProtectionFailurePreservesPreviousPublication(t *testing.T) {
	e, resources := publicationRecoveryEngine(t)
	if err := e.Rebuild(publicationRecoveryDocs(), ports.Meta{Basis: "before", Cause: "observation"}); err != nil {
		t.Fatal(err)
	}
	old, _, err := e.loadControl()
	if err != nil {
		t.Fatal(err)
	}
	session, err := e.BeginRebuild(ports.Meta{Basis: "after", Cause: "observation"})
	if err != nil {
		t.Fatal(err)
	}
	if err := session.Append(publicationRecoveryDocs()); err != nil {
		t.Fatal(err)
	}
	candidate := session.(*rebuildSession).physicalIndex
	e.http.Transport = rejectProtectionRecord{next: resources}
	if err := session.Commit(); kernel.CodeOf(err) != kernel.ErrTemporaryUnavailable {
		t.Fatalf("publication accepted without durable old-resource protection: %v", err)
	}
	current, _, err := e.loadControl()
	if err != nil || current.ActiveIndex != old.ActiveIndex || current.Basis != "before" || len(current.ObservationDeadlines) != 0 {
		t.Fatalf("failed protection damaged accepted publication: %#v %v", current, err)
	}
	if status, _, err := e.do(http.MethodHead, "/"+url.PathEscape(candidate), nil); err != nil || status != http.StatusNotFound {
		t.Fatalf("unpublished candidate was not cleaned: HTTP %d %v", status, err)
	}
}

func TestLiveOpenSearchReplacementProtectionSurvivesReopen(t *testing.T) {
	e, resources := publicationRecoveryEngine(t)
	if err := e.Rebuild(publicationRecoveryDocs(), ports.Meta{Basis: "before", Cause: "observation"}); err != nil {
		t.Fatal(err)
	}
	old, _, err := e.loadControl()
	if err != nil {
		t.Fatal(err)
	}
	if err := e.Rebuild(publicationRecoveryDocs(), ports.Meta{Basis: "after", Cause: "observation"}); err != nil {
		t.Fatal(err)
	}
	accepted, _, err := e.loadControl()
	if err != nil || accepted.ObservationDeadlines[old.Generation] <= 0 {
		t.Fatalf("replacement deadline not accepted: %#v %v", accepted, err)
	}
	if err := e.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(Config{URL: e.base, Namespace: e.namespace, PrimaryShards: 1})(t.TempDir(), e.repository)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	fresh := reopened.(*openSearchEngine)
	fresh.http.Transport = resources
	current, _, err := fresh.loadControl()
	if err != nil || current.ObservationDeadlines[old.Generation] != accepted.ObservationDeadlines[old.Generation] {
		t.Fatalf("reopen changed original deadline: %#v %v", current, err)
	}
	path := "/" + fresh.controlIndexPath() + "/_doc/" + url.PathEscape(fresh.controlID+"-retained-"+old.Generation)
	status, body, err := fresh.do(http.MethodGet, path, nil)
	var record struct {
		Source controlDoc `json:"_source"`
	}
	if err != nil || status != http.StatusOK || json.Unmarshal(body, &record) != nil {
		t.Fatalf("replacement lost durable protection: HTTP %d %s %v", status, body, err)
	}
	if record.Source.State != "RETAINED_GENERATION" || record.Source.ActiveIndex != old.ActiveIndex || record.Source.Basis != "before" {
		t.Fatalf("protection lost original resource and basis: %#v", record.Source)
	}
	if count, err := fresh.countIndex(old.ActiveIndex); err != nil || count != 1 {
		t.Fatalf("replacement removed retained corpus: %d %v", count, err)
	}
	meta, err := fresh.LoadMeta()
	if err != nil || meta.Basis != "after" {
		t.Fatalf("reopen lost current corpus: %#v %v", meta, err)
	}
	// Repeating the same ownership proof is idempotent; a mismatched record
	// must stop publication rather than silently taking over another resource.
	session := &rebuildSession{engine: fresh, old: old, physicalIndex: "unused"}
	if err := session.preservePreviousGeneration(); err != nil {
		t.Fatal(err)
	}
	session.old.ActiveIndex += "-different"
	if err := session.preservePreviousGeneration(); kernel.CodeOf(err) != kernel.ErrPreconditionFailed {
		t.Fatalf("conflicting resource ownership accepted: %v", err)
	}
}
