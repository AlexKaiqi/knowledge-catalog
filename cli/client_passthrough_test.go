package cli_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"kc/cli"
	"kc/client"
)

func TestClientWorksWithLocalKCPassThroughServiceWithoutDelegation(t *testing.T) {
	home := t.TempDir()
	handler := cli.HTTPHandler(home)
	seenAs := make(chan string, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		seenAs <- request.Header.Get("X-Kc-As")
		handler.ServeHTTP(w, request)
	}))
	t.Cleanup(server.Close)
	kcClient, err := client.New(client.Config{BaseURL: server.URL, HTTPClient: server.Client()})
	if err != nil {
		t.Fatal(err)
	}
	_, err = kcClient.Login(context.Background(), client.LoginRequest{
		Identity: client.Identity{Principal: "agent:test"},
	})
	if err != nil {
		t.Fatal(err)
	}
	who, err := kcClient.IdentityService().WhoAmI(context.Background(), client.RequestOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if as := <-seenAs; as != "agent:test" {
		t.Fatalf("local pairing must send X-Kc-As: %q", as)
	}
	if who.Principal != "agent:test" || who.OnBehalfOf != "" {
		t.Fatalf("whoami: %#v", who)
	}
}
