package gitea

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestManagedUserCreatesSameUsernameAndRecoversLostResponse(t *testing.T) {
	var saved managedUserInfo
	creates := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/api/v1/users/kaiqidong" {
			if saved.ID == 0 {
				w.WriteHeader(404)
				return
			}
			_ = json.NewEncoder(w).Encode(saved)
			return
		}
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/admin/users" {
			t.Fatalf("unexpected user operation: %s %s", r.Method, r.URL.Path)
		}
		creates++
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["username"] != "kaiqidong" || body["login_name"] != "kaiqidong" || body["source_id"] != float64(7) || body["email"] != "kaiqidong@example.test" {
			t.Errorf("user provisioning lost identity fields: %v", body)
		}
		saved = managedUserInfo{ID: 31, Login: "kaiqidong", FullName: body["full_name"].(string)}
		w.WriteHeader(500) // Creation committed, but its success response was lost.
	}))
	defer server.Close()
	request := ManagedUserRequest{Origin: server.URL, Username: "kaiqidong", EmailDomain: "example.test", AuthSourceID: 7, AllocationID: "reserved-account"}
	backend, err := EnsureManagedUser(request, "server-only-token")
	if err != nil || backend != 31 {
		t.Fatalf("lost-response recovery: backend=%d err=%v", backend, err)
	}
	request.BackendID = backend
	if _, err := EnsureManagedUser(request, "server-only-token"); err != nil {
		t.Fatal(err)
	}
	if creates != 1 {
		t.Fatalf("retry duplicated account %d", creates)
	}
	saved.ID = 32
	if _, err := EnsureManagedUser(request, "server-only-token"); err == nil {
		t.Fatal("replacement account was adopted")
	}
}

func TestManagedUserRequiresOwnershipOrVerifiedBackendIdentity(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Fatal("existing account must never be modified")
		}
		_ = json.NewEncoder(w).Encode(managedUserInfo{ID: 41, Login: "kaiqidong", FullName: "Unrelated Existing Account"})
	}))
	defer server.Close()
	request := ManagedUserRequest{Origin: server.URL, Username: "kaiqidong", AllocationID: "new-reservation"}
	if _, err := EnsureManagedUser(request, "server-token"); err == nil {
		t.Fatal("MANAGED-NO-ACCOUNT-TAKEOVER: username alone claimed an unrelated account")
	}
	request.TrustedBackendID = 42
	if _, err := EnsureManagedUser(request, "server-token"); err == nil {
		t.Fatal("another authenticated subject claimed the username")
	}
	request.TrustedBackendID = 41
	if id, err := EnsureManagedUser(request, "server-token"); err != nil || id != 41 {
		t.Fatalf("verified same Gitea subject was refused: %d %v", id, err)
	}
}
