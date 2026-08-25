package gitea_test

import (
	"testing"

	"kc/snapshot"
	"kc/snapshot/gitea"
)

func TestParseDSN(t *testing.T) {
	ep, err := gitea.ParseDSN("http://127.0.0.1:3001/kc/public-core")
	if err != nil {
		t.Fatal(err)
	}
	if ep.Origin != "http://127.0.0.1:3001" || ep.API != "http://127.0.0.1:3001/api/v1" || ep.Owner != "kc" || ep.Name != "public-core" {
		t.Fatalf("%#v", ep)
	}
	ep, err = gitea.ParseDSN("https://git.example.com/gitea/acme/core")
	if err != nil {
		t.Fatal(err)
	}
	if ep.Origin != "https://git.example.com/gitea" || ep.Owner != "acme" || ep.Name != "core" {
		t.Fatalf("%#v", ep)
	}
	if _, err := gitea.ParseDSN("http://user:secret@127.0.0.1:3001/kc/core"); err == nil {
		t.Fatal("expected secret rejection")
	}
	if err := snapshot.RejectConfiguredSecret("gitea", "http://127.0.0.1:3001/kc/core", gitea.EnvToken); err != nil {
		t.Fatal(err)
	}
}
