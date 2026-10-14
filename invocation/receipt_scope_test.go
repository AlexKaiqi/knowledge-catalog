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
