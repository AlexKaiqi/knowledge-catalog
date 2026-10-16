package home

import (
	"os"
	"path/filepath"
	"testing"

	"kc/internal/bindingfixture"
	"kc/internal/testkit"
	"kc/kernel"
	objectmodel "kc/object"
	repofile "kc/object/format"
	"kc/object/writer"
	"kc/snapshot"
	"kc/systemrepo"
)

func descriptorReadOnlyFixture(t *testing.T) (testkit.Setup, snapshot.Store, snapshot.Store) {
	t.Helper()
	s := testkit.NewSetup(t, "kr://descriptor/target")
	system := testkit.MakeTreeStore(t, string(objectmodel.SystemRepositoryID))
	if _, err := systemrepo.PublishSystem(system); err != nil {
		t.Fatal(err)
	}
	wrapped, err := readOnlySystemAuthority(system)
	if err != nil {
		t.Fatal(err)
	}
	if _, writable := snapshot.TreeStoreOf(wrapped); writable {
		t.Fatal("runtime System authority exposed literal writes")
	}
	if err := s.Store.Add(wrapped); err != nil {
		t.Fatal(err)
	}
	shared := addDescriptorReadSource(t, s, "kr://descriptor/source")
	return s, wrapped, shared
}

func addDescriptorReadSource(t *testing.T, s testkit.Setup, id string) snapshot.Store {
	t.Helper()
	source := testkit.MakeTreeStore(t, id)
	if err := s.Store.Add(source); err != nil {
		t.Fatal(err)
	}
	at := testkit.MustHead(t, source, snapshot.DefaultRef)
	if _, err := s.Writer.Commit("seed-"+id, objectmodel.ChangeSet{TargetRepository: source.ID(), TargetRef: snapshot.DefaultRef,
		BaseCommit: at, ExpectedTargetCommit: at,
		Operations: []objectmodel.Operation{bindingfixture.PutDescriptor("resource/stats", bindingfixture.Descriptor("https://stats.example"))}}); err != nil {
		t.Fatal(err)
	}
	return source
}

func TestWriterDescriptorResolutionReadsSystemWithoutAcquiringWriteCapability(t *testing.T) {
	s, system, _ := descriptorReadOnlyFixture(t)
	systemHead := testkit.MustHead(t, system, snapshot.DefaultRef)
	paths, err := repofile.ReadObjectLocator(system.(snapshot.TreeReader), "resource/stats", systemHead)
	if err != nil || len(paths) != 0 {
		t.Fatalf("a normal missing System locator must remain an absent dependency: %v %v", paths, err)
	}
	dir := t.TempDir()
	draft := "---\nobject_id: table/orders\naspect_name: stats\nkind: Aspect\nvalueSource:\n  kind: binding\n  binding:\n    descriptorRef: resource/stats\n---\n"
	if err := os.WriteFile(filepath.Join(dir, "stats.yaml"), []byte(draft), 0600); err != nil {
		t.Fatal(err)
	}
	preview, err := writer.Ingest(dir, s.RepositoryID, s.RootCommitID)
	if err != nil || len(preview.ChangeSet.Operations) != 1 || preview.ChangeSet.Operations[0].Value != nil {
		t.Fatalf("external declaration preview: %#v %v", preview, err)
	}
	if testkit.MustHead(t, s.Repo, snapshot.DefaultRef) != s.RootCommitID {
		t.Fatal("preview mutated the target")
	}
	receipt, err := s.Writer.Commit("cross-repository-descriptor", preview.ChangeSet)
	if err != nil {
		t.Fatalf("cross-repository declaration could not read the runtime System authority: %v", err)
	}
	result, err := s.Reader.ReadAddress(s.RepositoryID, preview.ChangeSet.Operations[0].Address, receipt.Result.NewCommit)
	if err != nil || len(result.Declarations) != 1 || result.Declarations[0].ValueSource == nil || result.Declarations[0].ValueSource.Binding.DescriptorRef != "resource/stats" {
		t.Fatalf("published declaration did not retain its descriptor: %#v %v", result, err)
	}
	if testkit.MustHead(t, system, snapshot.DefaultRef) != systemHead {
		t.Fatal("descriptor resolution mutated System authority")
	}
	if _, writable := snapshot.TreeStoreOf(system); writable || kernel.CodeOf(system.CreateRef("forbidden", systemHead)) != kernel.ErrForbidden {
		t.Fatal("descriptor resolution enabled runtime System writes")
	}
}

func TestWriterDescriptorResolutionPreservesUnknownHoldersAndAmbiguity(t *testing.T) {
	for _, fault := range []string{"system locator", "second source head", "second source empty head", "second source body", "second source missing body", "duplicate holder"} {
		t.Run(fault, func(t *testing.T) {
			s, system, _ := descriptorReadOnlyFixture(t)
			if fault != "system locator" {
				addDescriptorReadSource(t, s, "kr://descriptor/source-z")
			}
			want := kernel.ErrTemporaryUnavailable
			if fault == "duplicate holder" {
				want = kernel.ErrObjectIDConflict
			}
			if fault == "second source empty head" {
				want = kernel.ErrPreconditionFailed
			}
			if fault == "second source missing body" {
				want = kernel.ErrObjectRefUnresolved
			}
			s.Store.SetWrap(func(source snapshot.Store) snapshot.Store {
				if fault == "system locator" && source.ID() == system.ID() {
					return descriptorReadFault{Store: source, TreeReader: source.(snapshot.TreeReader),
						failedPath: repofile.ObjectLocatorPath("resource/stats")}
				}
				if source.ID() != "kr://descriptor/source-z" {
					return source
				}
				switch fault {
				case "second source head":
					return descriptorReadFault{Store: source, TreeReader: source.(snapshot.TreeReader), failHead: true}
				case "second source empty head":
					return descriptorReadFault{Store: source, TreeReader: source.(snapshot.TreeReader), emptyHead: true}
				case "second source body":
					return descriptorReadFault{Store: source, TreeReader: source.(snapshot.TreeReader),
						failedPath: repofile.DefaultPath(objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "resource/stats"}, "")}
				case "second source missing body":
					return descriptorReadFault{Store: source, TreeReader: source.(snapshot.TreeReader), missingBody: true,
						failedPath: repofile.DefaultPath(objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "resource/stats"}, "")}
				}
				return source
			})
			_, err := s.Writer.Commit("reject-unknown-descriptor", objectmodel.ChangeSet{TargetRepository: s.RepositoryID, TargetRef: snapshot.DefaultRef,
				BaseCommit: s.RootCommitID, ExpectedTargetCommit: s.RootCommitID,
				Operations: []objectmodel.Operation{bindingfixture.PutAspect("table/orders", "stats", "", "resource/stats")}})
			if kernel.CodeOf(err) != want {
				t.Fatalf("unknown holder or conflict became a unique descriptor: got %v, want %s", err, want)
			}
			if testkit.MustHead(t, s.Repo, snapshot.DefaultRef) != s.RootCommitID {
				t.Fatal("rejected resolution advanced the target")
			}
		})
	}
}

type descriptorReadFault struct {
	snapshot.Store
	snapshot.TreeReader
	failedPath  string
	failHead    bool
	emptyHead   bool
	missingBody bool
}

func (s descriptorReadFault) ReadFile(path string, commit kernel.CommitID) ([]byte, error) {
	if path == s.failedPath {
		if s.missingBody {
			return nil, kernel.Fail(kernel.ErrObjectRefUnresolved, "descriptor body is missing behind its locator")
		}
		return nil, kernel.Fail(kernel.ErrTemporaryUnavailable, "descriptor source read unavailable")
	}
	return s.TreeReader.ReadFile(path, commit)
}

func (s descriptorReadFault) Head(ref string) (kernel.CommitID, error) {
	if s.failHead {
		return "", kernel.Fail(kernel.ErrTemporaryUnavailable, "descriptor source Head unavailable")
	}
	if s.emptyHead {
		return "", nil
	}
	return s.Store.Head(ref)
}
