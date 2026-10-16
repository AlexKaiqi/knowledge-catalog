package cli_test

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// The inputs come from commands printed in the public guide and files delivered
// by the Server. The private fixture tree is used only to publish the world.
func TestPublicPublicationGuideReusesDeliveredExample(t *testing.T) {
	t.Run("verbatim-example", func(t *testing.T) {
		testPublicPublicationGuide(t, false)
	})
	t.Run("supplier-adaptation", func(t *testing.T) {
		testPublicPublicationGuide(t, true)
	})
}

func testPublicPublicationGuide(t *testing.T, adapt bool) {
	t.Helper()
	world := readExampleWorld(t)
	const target = "supplier-practice"
	world.Repositories = append(world.Repositories, struct {
		ID        string `json:"id"`
		Provision string `json:"provision"`
	}{target, "managed"})
	d := openExampleDeployment(t, world)
	d.publish(t, world, true)
	const supplier = "agent:new-supplier"
	body(t, d.as(t, d.operator, "grant", "add", "--principal", supplier, "--repo", target,
		"--action", "object.read,object.schema.read,writer.preview,writer.commit,writer.receipt.read"))
	guide := asMap(t, body(t, d.as(t, supplier, "read", "urn:kc:object:kc/guide/first-publication#/guide", "--repo", "kr://kc/system")))
	text := asMap(t, guide["value"])["body"].(string)
	drafts := downloadGuideExamples(t, d, supplier, text)
	original := asMap(t, body(t, d.as(t, supplier, "read", "urn:kc:object:product/qinghe.osmanthus-oolong#/definition",
		"--dataset", "autumn-launch-kit", "--revision", "3")))
	expectedValue := cloneJSONMap(t, asMap(t, original["value"]))
	reference := "urn:kc:object:product/qinghe.osmanthus-oolong#/definition"
	if adapt {
		// Copying a format does not make the example's business facts our own.
		path := filepath.Join(drafts, "products", "osmanthus-oolong", "definition.yaml")
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		updated := bytes.Replace(raw, []byte("product/qinghe.osmanthus-oolong"), []byte("product/supplier.jasmine"), 1)
		updated = bytes.Replace(updated, []byte("桂花乌龙"), []byte("茉莉茶"), 1)
		if bytes.Equal(raw, updated) {
			t.Fatal("the public example must contain the product being adapted")
		}
		if err := os.WriteFile(path, updated, 0600); err != nil {
			t.Fatal(err)
		}
		reference = "urn:kc:object:product/supplier.jasmine#/definition"
		expectedValue["name"] = "茉莉茶"
	}
	head := asMap(t, body(t, d.as(t, supplier, "writer", "head", "--repo", target)))
	base := head["commit"].(string)
	body(t, d.as(t, supplier, "diff", "--repo", target, "--base", base, "--dir", drafts))
	receipt := asMap(t, body(t, d.as(t, supplier, "writer", "commit", "--repo", target,
		"--command-id", "learn-from-public-example", "--base", base, "--dir", drafts)))
	commit, ok := asMap(t, receipt["result"])["newCommit"].(string)
	if !ok || commit == "" {
		t.Fatalf("publication needs a fixed readback basis: %#v", receipt)
	}
	recovered := asMap(t, body(t, d.as(t, supplier, "writer", "receipt", "--command-id", "learn-from-public-example")))
	if asMap(t, asMap(t, recovered["receipt"])["result"])["newCommit"] != commit {
		t.Fatalf("receipt lookup must preserve the publication basis: %#v", recovered)
	}
	read := asMap(t, body(t, d.as(t, supplier, "read", reference,
		"--repo", target, "--commit", commit)))
	if read["commit"] != commit || read["reference"] != reference || !reflect.DeepEqual(read["value"], expectedValue) {
		t.Fatalf("public example must preserve the complete input at the accepted commit: %#v", read)
	}
	schema := asMap(t, body(t, d.as(t, supplier, "read", "urn:kc:object:schema/product.definition",
		"--repo", target, "--commit", commit)))
	sourceSchema := asMap(t, body(t, d.as(t, supplier, "read", "urn:kc:object:schema/product.definition",
		"--dataset", "autumn-launch-kit", "--revision", "3")))
	if schema["commit"] != commit || !reflect.DeepEqual(schema["value"], sourceSchema["value"]) {
		t.Fatalf("reused Schema must preserve its complete public definition: %#v", schema)
	}
	expectCode(t, d.as(t, supplier, "read", "urn:kc:object:product/qinghe.osmanthus-oolong#/definition", "--repo", "product"), "FORBIDDEN")
	after := asMap(t, body(t, d.as(t, supplier, "read", "urn:kc:object:product/qinghe.osmanthus-oolong#/definition",
		"--dataset", "autumn-launch-kit", "--revision", "3")))
	if !reflect.DeepEqual(after, original) {
		t.Fatalf("the published example must remain unchanged: before=%#v after=%#v", original, after)
	}
}

func downloadGuideExamples(t *testing.T, d *exampleDeployment, principal, guide string) string {
	t.Helper()
	drafts := t.TempDir()
	downloads := 0
	discovered := map[string]bool{}
	for _, line := range strings.Split(guide, "\n") {
		line = strings.TrimSpace(line)
		if line == "kc show" || strings.HasPrefix(line, "kc dataset show ") || strings.HasPrefix(line, "kc files list ") {
			result := asMap(t, body(t, d.as(t, principal, strings.Fields(strings.TrimPrefix(line, "kc "))...)))
			if entries, ok := result["entries"].([]any); ok {
				for _, entry := range entries {
					discovered[asMap(t, entry)["path"].(string)] = true
				}
			}
			continue
		}
		if !strings.HasPrefix(line, "kc files read ") || !strings.Contains(line, "--out ./drafts/") {
			continue
		}
		args := strings.Fields(strings.TrimPrefix(line, "kc "))
		for i := range args {
			if args[i] == "--path" && i+1 < len(args) && !discovered[args[i+1]] {
				t.Fatalf("guide must discover a file before downloading it: %s", line)
			}
			if args[i] != "--out" || i+1 >= len(args) {
				continue
			}
			rel := strings.TrimPrefix(args[i+1], "./drafts/")
			if !filepath.IsLocal(rel) {
				t.Fatalf("guide download must stay in drafts: %s", line)
			}
			args[i+1] = filepath.Join(drafts, filepath.FromSlash(rel))
			if err := os.MkdirAll(filepath.Dir(args[i+1]), 0700); err != nil {
				t.Fatal(err)
			}
		}
		body(t, d.as(t, principal, args...))
		downloads++
	}
	if downloads != 2 {
		t.Fatalf("public guide must deliver its complete Schema and instance inputs; found %d downloads", downloads)
	}
	return drafts
}
