package cli

import (
	apphome "kc/home"
	inv "kc/invocation"
	"testing"

	"kc/kernel"
	"kc/snapshot"
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
	if err := writeAllow(dir, inv.AllowFile{Version: inv.AllowVersion, Rules: []inv.AllowRule{{ID: "receipt", Principal: "kaiqidong", Repo: "kr://owned/repo", Ref: string(snapshot.DefaultRef), Actions: []string{"writer.receipt.read"}}}}); err != nil {
		t.Fatal(err)
	}
	cx := &invocation{Command: "writer-receipt", Home: dir, WS: &apphome.Home{Commands: ledger}, Flags: map[string]FlagValue{"as": "kaiqidong", "command-id": "published-command"}}
	if err := inv.Authorize(dir, "writer.receipt.read", inv.AuthorizationFlags(cx), nil); err != nil {
		t.Fatalf("author cannot query own published operation: %v", err)
	}
	cx.Flags["repo"] = "kr://spoofed/repo"
	if actual := flagString(inv.AuthorizationFlags(cx), "repo"); actual != "kr://owned/repo" {
		t.Fatalf("receipt scope trusts request instead of durable command: %q", actual)
	}
	cx.Flags["as"] = "other-user"
	if err := inv.Authorize(dir, "writer.receipt.read", inv.AuthorizationFlags(cx), nil); kernel.CodeOf(err) != kernel.ErrForbidden {
		t.Fatalf("other user read receipt: %v", err)
	}
	cx.Flags["as"] = "kaiqidong"
	if err := writeAllow(dir, inv.AllowFile{Version: inv.AllowVersion, Rules: []inv.AllowRule{}}); err != nil {
		t.Fatal(err)
	}
	if err := inv.Authorize(dir, "writer.receipt.read", inv.AuthorizationFlags(cx), nil); kernel.CodeOf(err) != kernel.ErrForbidden {
		t.Fatalf("revocation ignored: %v", err)
	}
}
