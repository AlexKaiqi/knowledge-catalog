package session

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	kcclient "kc/client/auth"
	"kc/kernel"
)

func TestSessionClientRereadsLocalLoginAndLogoutOnEveryRequest(t *testing.T) {
	isolateLoginConfig(t)
	var principals []string
	var principalsMu sync.Mutex
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		principalsMu.Lock()
		principals = append(principals, r.Header.Get("X-Kc-As"))
		principalsMu.Unlock()
		_, _ = io.WriteString(w, `{"principal":"verified"}`)
	}))
	defer server.Close()
	if err := persistLocalSession(localSession{Server: server.URL, Principal: "alice"}); err != nil {
		t.Fatal(err)
	}
	client, err := newClient(context.Background(), server.URL, Selection{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.IdentityService().WhoAmI(context.Background(), kcclient.RequestOptions{}); err != nil {
		t.Fatal(err)
	}
	if err := persistLocalSession(localSession{Server: server.URL, Principal: "bob"}); err != nil {
		t.Fatal(err)
	}
	if _, err := client.IdentityService().WhoAmI(context.Background(), kcclient.RequestOptions{}); err != nil {
		t.Fatal(err)
	}
	if _, err := Logout(context.Background(), server.URL); err != nil {
		t.Fatal(err)
	}
	if _, err := client.IdentityService().WhoAmI(context.Background(), kcclient.RequestOptions{}); kernel.CodeOf(err) != kernel.ErrUnauthenticated {
		t.Fatalf("logout left long-running client authenticated: %v", err)
	}
	principalsMu.Lock()
	defer principalsMu.Unlock()
	if len(principals) != 2 || principals[0] != "alice" || principals[1] != "bob" {
		t.Fatalf("long-running client retained original login: %v", principals)
	}
}
func TestSessionClientNeverSendsSelectedServiceCredentialsToAnotherTarget(t *testing.T) {
	isolateLoginConfig(t)
	var requests atomic.Int32
	foreign := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { requests.Add(1) }))
	defer foreign.Close()
	client, err := newClient(context.Background(), "http://selected.invalid/service", Selection{Principal: "alice"})
	if err != nil {
		t.Fatal(err)
	}
	request, _ := http.NewRequest(http.MethodGet, foreign.URL+"/identity/v1/whoami", nil)
	if _, err := client.Do(context.Background(), "kc", request); kernel.CodeOf(err) != kernel.ErrUnauthenticated || requests.Load() != 0 {
		t.Fatalf("service credential crossed endpoint boundary: requests=%d %v", requests.Load(), err)
	}
}
func TestSessionLocalLoginRejectsCredentialRedirect(t *testing.T) {
	isolateLoginConfig(t)
	var foreignRequests atomic.Int32
	foreign := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		foreignRequests.Add(1)
		_, _ = io.WriteString(w, `{"principal":"alice"}`)
	}))
	defer foreign.Close()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/identity/v1/auth" {
			_, _ = io.WriteString(w, `{"mode":"local","localAssertion":true}`)
			return
		}
		http.Redirect(w, r, foreign.URL+"/whoami", http.StatusFound)
	}))
	defer server.Close()
	if _, err := Login(context.Background(), LoginOptions{Server: server.URL, Mode: "local", Principal: "alice"}, LoginInteraction{}); err == nil || foreignRequests.Load() != 0 {
		t.Fatalf("local credential followed redirect: requests=%d %v", foreignRequests.Load(), err)
	}
	if _, ok := loadLocalSession(server.URL); ok {
		t.Fatal("redirected login was persisted")
	}
}
func TestSessionConcurrentRefreshUsesOneVerifiedReplacement(t *testing.T) {
	isolateLoginConfig(t)
	var refreshes atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/identity/v1/token":
			refreshes.Add(1)
			_ = json.NewEncoder(w).Encode(map[string]any{"accessToken": "fresh", "refreshToken": "next", "expiresIn": 3600})
		case "/identity/v1/whoami":
			if r.Header.Get("Authorization") != "Bearer fresh" || r.Header.Get("X-Kc-As") != "" {
				t.Error("refresh mixed identity proofs")
			}
			_, _ = io.WriteString(w, `{"principal":"alice"}`)
		default:
			t.Errorf("unexpected refresh endpoint %s", r.URL.Path)
		}
	}))
	defer server.Close()
	if err := persistTokenLogin(taihuSession{Server: server.URL, Principal: "alice", AccessToken: "old", RefreshToken: "refresh", ExpiresAt: time.Now().Add(-time.Minute)}); err != nil {
		t.Fatal(err)
	}
	var group sync.WaitGroup
	for i := 0; i < 12; i++ {
		group.Add(1)
		go func() {
			defer group.Done()
			client, err := newClient(context.Background(), server.URL, Selection{})
			if err != nil {
				t.Error(err)
				return
			}
			if _, err := client.IdentityService().WhoAmI(context.Background(), kcclient.RequestOptions{}); err != nil {
				t.Error(err)
			}
		}()
	}
	group.Wait()
	if refreshes.Load() != 1 {
		t.Fatalf("concurrent callers refreshed %d times", refreshes.Load())
	}
}
