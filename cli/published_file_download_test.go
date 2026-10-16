package cli_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPublishedImageReferenceDownloadsThroughPublicFileCommand(t *testing.T) {
	f := newPublishedObjectFixture(t, false)
	f.publish(t, "image")
	commit := publishedCommit(t, asMap(t, body(t, f.provide(t, "writer", "put", "urn:kc:object:note/download-check", "--repo", f.repository,
		"--command-id", "advance-description-repository", "--value", `{"purpose":"resource version remains independent"}`))))
	read := asMap(t, body(t, f.consume(t, "read", "urn:kc:object:image/tea-tin#/definition", "--repo", f.repository, "--commit", commit)))
	definition := asMap(t, read["value"])
	resource := asMap(t, definition["resources"].([]any)[0])
	if definition["kind"] != "image" || resource["commit"] == commit {
		t.Fatalf("resource declaration must retain its independent image version: %#v", definition)
	}
	if !strings.Contains(definition["summary"].(string), "kc help files read") {
		t.Fatal("image definition does not point its consumer to the file help")
	}
	help := kc(t.TempDir(), "help", "files", "read")
	if help.Status != 0 || !strings.Contains(help.Stdout, "--repo <id> --commit <commit> --path <file>") {
		t.Fatalf("public help does not expose the declared coordinates: %#v", help)
	}
	target := filepath.Join(t.TempDir(), "tea-tin.png")
	args := []string{"files", "read", "--repo", resource["repository"].(string), "--commit", resource["commit"].(string), "--path", resource["path"].(string), "--out", target}
	expectCode(t, f.consume(t, args...), "FORBIDDEN")
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Fatalf("object.read or the declaration granted image bytes: %v", err)
	}
	grant := asMap(t, body(t, f.govern(t, "grant", "add", "--principal", "agent:object-consumer", "--repo", f.resourceRepository, "--ref", f.resourceCommit, "--action", "file.read")))
	receipt := asMap(t, body(t, f.consume(t, args...)))
	expected, err := os.ReadFile(filepath.Join(publishedObjectMaterials(), "resources", "media", "tea-tin.png"))
	if err != nil {
		t.Fatal(err)
	}
	actual, err := os.ReadFile(target)
	if err != nil || !bytes.Equal(actual, expected) || receipt["repository"] != resource["repository"] || receipt["commit"] != resource["commit"] || receipt["path"] != resource["path"] || receipt["dataset"] != nil {
		t.Fatalf("public download changed bytes or resource basis: %#v %v", receipt, err)
	}
	body(t, f.govern(t, "grant", "remove", "--id", grant["id"].(string)))
	args[len(args)-1] = filepath.Join(t.TempDir(), "revoked.png")
	expectCode(t, f.consume(t, args...), "FORBIDDEN")
	if _, err := os.Stat(args[len(args)-1]); !os.IsNotExist(err) {
		t.Fatalf("revoked file read published a local file: %v", err)
	}
}
