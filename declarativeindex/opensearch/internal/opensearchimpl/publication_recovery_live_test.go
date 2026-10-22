package opensearchimpl

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync/atomic"
	"testing"

	retrieval "kc/discovery"
	ports "kc/discovery/projection"
	"kc/kernel"
	objectmodel "kc/object"
)

type publicationAckLoss struct {
	next           http.RoundTripper
	path           string
	lost           atomic.Bool
	blockAuthority atomic.Bool
	reject         bool
	afterAdoption  func() error
}

func (l *publicationAckLoss) RoundTrip(request *http.Request) (*http.Response, error) {
	if l.lost.Load() && l.blockAuthority.Load() && request.Method == http.MethodGet && request.URL.Path == l.path {
		return nil, fmt.Errorf("injected control authority outage after publication")
	}
	ready := false
	if request.Method == http.MethodPut && request.URL.Path == l.path && request.GetBody != nil {
		body, err := request.GetBody()
		if err != nil {
			return nil, err
		}
		var control controlDoc
		err = json.NewDecoder(body).Decode(&control)
		_ = body.Close()
		if err != nil {
			return nil, err
		}
		ready = control.State == ports.ProjectionStateReady && control.Basis == "after"
	}
	if ready && l.reject {
		return &http.Response{StatusCode: http.StatusConflict, Body: io.NopCloser(strings.NewReader(`{"error":"injected CAS rejection"}`)), Header: http.Header{}, Request: request}, nil
	}
	response, err := l.next.RoundTrip(request)
	if err == nil && ready && response.StatusCode < 300 && l.lost.CompareAndSwap(false, true) {
		_, _ = io.Copy(io.Discard, response.Body)
		_ = response.Body.Close()
		if l.afterAdoption != nil {
			if err := l.afterAdoption(); err != nil {
				return nil, err
			}
		}
		return nil, fmt.Errorf("injected response loss after actual READY adoption")
	}
	return response, err
}

func TestLiveOpenSearchReplacedPublicationIsNotProofOfNonAdoption(t *testing.T) {
	e, resources := publicationRecoveryEngine(t)
	if err := e.Rebuild(publicationRecoveryDocs(), ports.Meta{Basis: "before"}); err != nil {
		t.Fatal(err)
	}
	old, _, err := e.loadControl()
	if err != nil {
		t.Fatal(err)
	}
	adopted := ""
	loss := &publicationAckLoss{next: resources, path: "/" + e.controlIndexPath() + "/_doc/" + url.PathEscape(e.controlID)}
	loss.afterAdoption = func() error {
		current, version, err := e.loadControl()
		if err != nil {
			return err
		}
		adopted = current.ActiveIndex
		// Another accepted publication replaces the just-adopted pointer
		// before the original publisher can recover its lost confirmation.
		_, err = e.putControl(old, version)
		return err
	}
	e.http.Transport = loss
	if err := e.Rebuild(publicationRecoveryDocs(), ports.Meta{Basis: "after"}); kernel.CodeOf(err) != kernel.ErrTemporaryUnavailable {
		t.Fatalf("replaced publication was guessed to be known: %v", err)
	}
	if adopted == "" || !loss.lost.Load() {
		t.Fatal("publication/replacement sequence did not execute")
	}
	count, err := e.countIndex(adopted)
	if err != nil || count != 1 {
		t.Fatalf("possibly retained generation was deleted after replacement: %d %v", count, err)
	}
	current, _, err := e.loadControl()
	if err != nil || current.ActiveIndex != old.ActiveIndex {
		t.Fatalf("uncertain recovery rewrote the current publication: %#v %v", current, err)
	}
}

func publicationRecoveryEngine(t *testing.T) (*openSearchEngine, *namespaceResources) {
	t.Helper()
	endpoint := strings.TrimRight(os.Getenv("KC_TEST_OPENSEARCH_URL"), "/")
	if endpoint == "" {
		t.Fatal("publication recovery requires the existing real OpenSearch endpoint")
	}
	t.Setenv(EnvEmbeddingModel, "")
	identity := sha256.Sum256([]byte(t.TempDir()))
	namespace := "publication-recovery-" + hex.EncodeToString(identity[:8])
	resources := &namespaceResources{endpoint: endpoint, indices: map[string]bool{}, documents: map[string]bool{}}
	t.Cleanup(func() { resources.cleanup(t) })
	engine, err := Open(Config{URL: endpoint, Namespace: namespace, PrimaryShards: 1})(t.TempDir(), "kr://publication/recovery")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = engine.Close() })
	e := engine.(*openSearchEngine)
	e.http.Transport = resources
	return e, resources
}

func publicationRecoveryDocs() []ports.CompiledDoc {
	return []ports.CompiledDoc{{ObjectID: "article/one", Kind: objectmodel.KindEntity, Text: "runbook", ObjectDigest: kernel.CanonicalDigest("runbook")}}
}

func TestLiveOpenSearchPublicationRecoversLostAcceptanceConfirmation(t *testing.T) {
	for _, warm := range []bool{false, true} {
		t.Run(fmt.Sprintf("warm=%t", warm), func(t *testing.T) {
			e, resources := publicationRecoveryEngine(t)
			if warm {
				if err := e.Rebuild(publicationRecoveryDocs(), ports.Meta{Basis: "before"}); err != nil {
					t.Fatal(err)
				}
			}
			loss := &publicationAckLoss{next: resources, path: "/" + e.controlIndexPath() + "/_doc/" + url.PathEscape(e.controlID)}
			e.http.Transport = loss
			intended := ports.Meta{Basis: "after"}
			if !warm {
				intended.SchemaAdoptions = []retrieval.SchemaAdoption{}
			}
			err := e.Rebuild(publicationRecoveryDocs(), intended)
			if !loss.lost.Load() {
				t.Fatal("test did not discard an actually accepted control response")
			}
			meta, readErr := e.LoadMeta()
			if readErr != nil || meta.State != ports.ProjectionStateReady || meta.Basis != "after" {
				t.Fatalf("accepted generation was damaged after response loss: %#v %v; rebuild=%v", meta, readErr, err)
			}
			control, _, readErr := e.loadControl()
			if readErr != nil {
				t.Fatal(readErr)
			}
			count, readErr := e.countIndex(control.ActiveIndex)
			if readErr != nil || count != 1 {
				t.Fatalf("accepted physical data was deleted: count=%d error=%v", count, readErr)
			}
			if err != nil {
				t.Fatalf("authoritative acceptance proof was not recovered: %v", err)
			}
		})
	}
}

func TestLiveOpenSearchUnknownPublicationRetainsDataAndRecoveryRecord(t *testing.T) {
	e, resources := publicationRecoveryEngine(t)
	loss := &publicationAckLoss{next: resources, path: "/" + e.controlIndexPath() + "/_doc/" + url.PathEscape(e.controlID)}
	loss.blockAuthority.Store(true)
	e.http.Transport = loss
	err := e.Rebuild(publicationRecoveryDocs(), ports.Meta{Basis: "after"})
	if !loss.lost.Load() || err == nil {
		t.Fatalf("unconfirmed publication was reported as known: lost=%v err=%v", loss.lost.Load(), err)
	}
	loss.blockAuthority.Store(false)
	meta, err := e.LoadMeta()
	if err != nil || meta.State != ports.ProjectionStateReady || meta.Basis != "after" {
		t.Fatalf("unconfirmed adopted data was discarded: %#v %v", meta, err)
	}
	recoveryPath := e.controlID + "-unconfirmed-" + meta.Generation
	status, body, err := e.do(http.MethodGet, "/"+e.controlIndexPath()+"/_doc/"+url.PathEscape(recoveryPath), nil)
	if err != nil || status != http.StatusOK {
		t.Fatalf("publication recovery record missing: status=%d body=%s err=%v", status, body, err)
	}
}

func TestLiveOpenSearchRejectedPublicationCleansOnlyItsCandidate(t *testing.T) {
	e, resources := publicationRecoveryEngine(t)
	if err := e.Rebuild(publicationRecoveryDocs(), ports.Meta{Basis: "before"}); err != nil {
		t.Fatal(err)
	}
	e.http.Transport = &publicationAckLoss{next: resources, path: "/" + e.controlIndexPath() + "/_doc/" + url.PathEscape(e.controlID), reject: true}
	session, err := e.BeginRebuild(ports.Meta{Basis: "after"})
	if err != nil {
		t.Fatal(err)
	}
	if err := session.Append(publicationRecoveryDocs()); err != nil {
		t.Fatal(err)
	}
	candidate := session.(*rebuildSession).physicalIndex
	if err := session.Commit(); kernel.CodeOf(err) != kernel.ErrNonFastForward {
		t.Fatalf("known CAS rejection changed: %v", err)
	}
	meta, err := e.LoadMeta()
	if err != nil || meta.Basis != "before" || meta.State != ports.ProjectionStateReady {
		t.Fatalf("rejected candidate damaged old serving generation: %#v %v", meta, err)
	}
	status, _, err := e.do(http.MethodHead, "/"+url.PathEscape(candidate), nil)
	if err != nil || status != http.StatusNotFound {
		t.Fatalf("definitively rejected candidate was retained: %d %v", status, err)
	}
}
