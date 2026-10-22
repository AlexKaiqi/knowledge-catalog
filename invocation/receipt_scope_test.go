package invocation

import (
	apphome "kc/home"
	"kc/invoke"
	"kc/kernel"
	"kc/snapshot"
	"testing"
)

func TestReceiptAuthorizationUsesDurableRepositoryScope(t *testing.T) {
	dir := t.TempDir()
	ledger, err := snapshot.NewCommandLedger(nil)
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = ledger.Execute("published-command", "digest", snapshot.CommandRequest{RepositoryID: "kr://owned/repo", TargetRef: string(snapshot.DefaultRef)}, func() (any, error) { return map[string]any{"newCommit": "published"}, nil })
	if err != nil {
		t.Fatal(err)
	}
	if err := WriteAllow(dir, AllowFile{Version: AllowVersion, Rules: []AllowRule{{ID: "receipt", Principal: "kaiqidong", Repo: "kr://owned/repo", Ref: string(snapshot.DefaultRef), Actions: []string{"writer.receipt.read"}}}}); err != nil {
		t.Fatal(err)
	}
	cx := &Invocation{Command: "writer-receipt", Home: dir, WS: &apphome.Home{Commands: ledger}, Flags: map[string]invoke.FlagValue{"as": "kaiqidong", "command-id": "published-command"}}
	if err := Authorize(dir, "writer.receipt.read", AuthorizationFlags(cx), nil); err != nil {
		t.Fatalf("author cannot query own published operation: %v", err)
	}
	cx.Flags["repo"] = "kr://spoofed/repo"
	if actual := invoke.FlagString(AuthorizationFlags(cx), "repo"); actual != "kr://owned/repo" {
		t.Fatalf("receipt scope trusts request instead of durable command: %q", actual)
	}
	cx.Flags["as"] = "other-user"
	if err := Authorize(dir, "writer.receipt.read", AuthorizationFlags(cx), nil); kernel.CodeOf(err) != kernel.ErrForbidden {
		t.Fatalf("other user read receipt: %v", err)
	}
	cx.Flags["as"] = "kaiqidong"
	if err := WriteAllow(dir, AllowFile{Version: AllowVersion, Rules: []AllowRule{}}); err != nil {
		t.Fatal(err)
	}
	if err := Authorize(dir, "writer.receipt.read", AuthorizationFlags(cx), nil); kernel.CodeOf(err) != kernel.ErrForbidden {
		t.Fatalf("revocation ignored: %v", err)
	}
}

func TestReceiptOmitsPrivateRecoveryMaterial(t *testing.T) {
	ledger, err := snapshot.NewCommandLedger(nil)
	if err != nil {
		t.Fatal(err)
	}
	request := snapshot.CommandRequest{RepositoryID: "kr://owned/repo", TreeChangeSet: []byte(`{"secret":"literal"}`), ApplicationRequest: []byte(`{"secret":"object"}`), Authority: "private-authority"}
	if _, _, err := ledger.Execute("private", "digest", request, func() (any, error) { return map[string]bool{"changed": false}, nil }); err != nil {
		t.Fatal(err)
	}
	cx := &Invocation{WS: &apphome.Home{Commands: ledger}, Flags: map[string]invoke.FlagValue{"command-id": "private"}}
	result, err := VerbReceipt(cx)
	if err != nil {
		t.Fatal(err)
	}
	entry := result.(snapshot.CommandEntry)
	if len(entry.Request.TreeChangeSet) != 0 || len(entry.Request.ApplicationRequest) != 0 || entry.Request.Authority != "" || entry.Execution != "" || entry.Phase != "" {
		t.Fatalf("private recovery material exposed: %+v", entry)
	}
	stored, ok := ledger.Lookup("private")
	if !ok || string(stored.Request.TreeChangeSet) != string(request.TreeChangeSet) || string(stored.Request.ApplicationRequest) != string(request.ApplicationRequest) || stored.Request.Authority != request.Authority {
		t.Fatal("receipt projection modified durable recovery material")
	}
	if string(entry.Receipt) != `{"changed":false}` {
		t.Fatalf("receipt result lost: %s", entry.Receipt)
	}
}
