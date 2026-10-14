package cli

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestSnapshotInvalidInputPrecedesLoginRefresh(t *testing.T) {
	t.Setenv("KC_CONFIG_DIR", t.TempDir())
	t.Setenv("KC_AUTH_TOKEN", "")
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		http.Error(w, "login must not refresh", http.StatusInternalServerError)
	}))
	defer server.Close()
	if err := persistTokenLogin(taihuSession{Server: server.URL, Principal: "consumer", AccessToken: "expired", RefreshToken: "refresh", ExpiresAt: time.Now().Add(-time.Minute)}); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{
		{"files", "read", "--dataset", "d", "--path", "x", "--length", "-1"},
		{"files", "list", "--dataset", "d", "--continuation", "cursor"},
		{"dataset", "show", "d", "--revision", "0"},
		{"grant", "update", "--id", "rule", "--file", "missing", "--expected-file", "missing"},
	} {
		result := Run(append(args, "--server", server.URL))
		if result.Status == 0 || !strings.Contains(result.Stdout, "USAGE_INVALID") {
			t.Fatalf("invalid input was replaced by authentication work: %#v", result)
		}
	}
	if calls != 0 {
		t.Fatalf("invalid input refreshed credentials %d times", calls)
	}
}

func TestSnapshotConsumerInputRejectsInvalidCoordinatesBeforeTransport(t *testing.T) {
	t.Setenv("KC_CONFIG_DIR", t.TempDir())
	t.Setenv("KC_AUTH_TOKEN", "")
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		http.Error(w, "must not reach transport", http.StatusInternalServerError)
	}))
	defer server.Close()
	for _, args := range [][]string{
		{"files", "list", "--dataset", "d", "--continuation", "cursor"},
		{"files", "list", "--dataset", "d", "--path", "../secret"},
		{"files", "list", "--dataset", "d", "--path", "/secret"},
		{"files", "list", "--dataset", "d", "--limit", "201"},
		{"files", "read", "--dataset", "d"},
		{"files", "read", "--dataset", "d", "--path", "a/../b"},
		{"files", "read", "--dataset", "d", "--path", "x", "--offset", "-1"},
		{"files", "read", "--dataset", "d", "--path", "x", "--length", "4194305"},
		{"files", "read", "--dataset", "d", "--path", "x", "--out", "target", "--offset", "0"},
		{"files", "read", "--dataset", "d", "--path", "x", "--repo", "other"},
		{"files", "read", "--dataset", "d", "--path", "x", "--path", "y"},
		{"dataset", "show", "d", "--revision", "0"},
		{"dataset", "show", "d", "--revision", "abc"},
		{"dataset", "show", "d", "--limit", "201"},
		{"read", "urn:kc:object:r:o", "--offset", "1"},
	} {
		result := Run(append(args, "--server", server.URL, "--as", "consumer"))
		if result.Status == 0 || !strings.Contains(result.Stdout, "USAGE_INVALID") {
			t.Errorf("invalid coordinates %v: %#v", args, result)
		}
	}
	if calls != 0 {
		t.Fatalf("invalid coordinates reached Server %d times", calls)
	}
}

func TestSnapshotNewEntrypointsRemainDiscoverableWithoutKcfsOwnership(t *testing.T) {
	root := Run([]string{"help"})
	for _, group := range []string{"dataset", "files", "grant"} {
		if !strings.Contains(root.Stdout, "kc help "+group) {
			t.Errorf("root lacks %s navigation", group)
		}
	}
	for group, leaves := range map[string][]string{"dataset": {"dataset show", "files list", "files read"}, "files": {"files list", "files read"}, "grant": {"grant update"}} {
		help := Run([]string{"help", group})
		for _, leaf := range leaves {
			if !strings.Contains(help.Stdout, "kc "+leaf) || Run(append([]string{"help"}, strings.Fields(leaf)...)).Status != 0 {
				t.Errorf("unreachable leaf %s through %s", leaf, group)
			}
		}
	}
}
