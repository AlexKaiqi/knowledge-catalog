package writer

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func writeIngestTree(t *testing.T, root string, files map[string]string) {
	t.Helper()
	for rel, body := range files {
		full := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
}

type tarMember struct {
	name string
	kind byte
	body string
}

func writeArchive(t *testing.T, path string, gz bool, members []tarMember) {
	t.Helper()
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	var tw *tar.Writer
	var zw *gzip.Writer
	if gz {
		zw = gzip.NewWriter(file)
		tw = tar.NewWriter(zw)
	} else {
		tw = tar.NewWriter(file)
	}
	for _, m := range members {
		header := &tar.Header{Name: m.name, Typeflag: m.kind, Mode: 0o644, Size: int64(len(m.body))}
		if m.kind == tar.TypeDir {
			header.Size, header.Mode = 0, 0o755
		}
		if m.kind == tar.TypeSymlink {
			header.Size, header.Linkname = 0, "elsewhere"
		}
		if err := tw.WriteHeader(header); err != nil {
			t.Fatal(err)
		}
		if header.Size > 0 {
			if _, err := tw.Write([]byte(m.body)); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if zw != nil {
		if err := zw.Close(); err != nil {
			t.Fatal(err)
		}
	}
}

func ingestFixture() map[string]string {
	files := map[string]string{
		".hidden.json":         `{"skip":true}`,
		".git/config.json":     `{"skip":true}`,
		"notes/.draft/x.json":  `{"skip":true}`,
		"README.md":            "# plain",
		"tables/orders.json":   "---\nobject_id: table/orders\n---\n{\"name\":\"orders\"}\n",
		"tables/items.json":    "---\nobject_id: table/items\naspect_name: schema\nkind: Aspect\n---\n{\"cols\":[1,2]}\n",
		"notes/raw.txt":        "raw text",
		"notes/value.json":     `{"n":1}`,
		"deep/a/b/c/leaf.yaml": "---\nobject_id: leaf/one\n---\nk: v\n",
	}
	for i := 0; i < 40; i++ {
		files[fmt.Sprintf("bulk/item-%02d.json", i)] = fmt.Sprintf("---\nobject_id: note/item-%02d\n---\n{\"i\":%d}\n", i, i)
	}
	return files
}

func archiveMembers(prefix string, files map[string]string) []tarMember {
	var members []tarMember
	if prefix != "" {
		members = append(members, tarMember{name: prefix + "/", kind: tar.TypeDir})
	}
	for rel, body := range files {
		name := rel
		if prefix != "" {
			name = prefix + "/" + rel
		}
		members = append(members, tarMember{name: name, kind: tar.TypeReg, body: body})
	}
	return members
}

func TestIngestArchiveMatchesDirectory(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "drafts")
	files := ingestFixture()
	writeIngestTree(t, dir, files)
	want, err := Ingest(dir, "kr://acme/r", "base")
	if err != nil {
		t.Fatal(err)
	}
	if len(want.ChangeSet.Operations) != len(files)-3 {
		t.Fatalf("hidden entries were not skipped: %d operations", len(want.ChangeSet.Operations))
	}
	wrapped := filepath.Join(root, "drafts.tar.gz")
	writeArchive(t, wrapped, true, archiveMembers("drafts", files))
	rooted := filepath.Join(root, "rooted.tar")
	writeArchive(t, rooted, false, archiveMembers("", files))
	dotted := filepath.Join(root, "dotted.tgz")
	writeArchive(t, dotted, true, archiveMembers(".", files))
	for name, input := range map[string]string{
		"gzip with subdir": wrapped + ArchiveSubdirSeparator + "drafts",
		"plain tar root":   rooted,
		"dot-prefixed tgz": dotted,
	} {
		got, err := Ingest(input, "kr://acme/r", "base")
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("%s: archive ingest differs from directory ingest", name)
		}
	}
}

func TestIngestArchiveRejectsUnsafeOrAmbiguousEntries(t *testing.T) {
	root := t.TempDir()
	cases := map[string][]tarMember{
		"escapes the archive root": {{name: "../evil.json", kind: tar.TypeReg, body: "{}"}},
		"is not a regular file":    {{name: "link.json", kind: tar.TypeSymlink}},
		"appears more than once":   {{name: "a.json", kind: tar.TypeReg, body: "{}"}, {name: "./a.json", kind: tar.TypeReg, body: "{}"}},
		"has no files under":       {{name: "other/a.json", kind: tar.TypeReg, body: "{}"}},
	}
	for want, members := range cases {
		path := filepath.Join(root, strings.ReplaceAll(want, " ", "-")+".tar")
		writeArchive(t, path, false, members)
		input := path
		if want == "has no files under" {
			input += ArchiveSubdirSeparator + "drafts"
		}
		if _, err := Ingest(input, "kr://acme/r", "base"); err == nil || !strings.Contains(err.Error(), want) {
			t.Fatalf("%s: err=%v", want, err)
		}
	}
	if _, err := Ingest(filepath.Join(root, "missing.tgz")+ArchiveSubdirSeparator+"x", "kr://acme/r", "base"); err == nil {
		t.Fatal("missing archive with subdir was accepted")
	}
}

func TestIngestReportsFirstErrorInDirectoryOrder(t *testing.T) {
	dir := t.TempDir()
	files := map[string]string{}
	for i := 0; i < 200; i++ {
		files[fmt.Sprintf("f-%03d.json", i)] = fmt.Sprintf("---\nobject_id: note/%d\n---\n{}\n", i)
	}
	files["f-050.json"] = "---\nobject_id: note/50\norigin: x\n---\n{}\n"
	files["f-150.json"] = "---\nobject_id: note/150\nvalue_source: x\n---\n{}\n"
	writeIngestTree(t, dir, files)
	for round := 0; round < 20; round++ {
		_, err := Ingest(dir, "kr://acme/r", "base")
		if err == nil || !strings.Contains(err.Error(), "origin is not a unit header") {
			t.Fatalf("round %d: first error in walk order not reported: %v", round, err)
		}
	}
}
