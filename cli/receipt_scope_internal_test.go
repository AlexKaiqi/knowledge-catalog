package cli

import (
	"testing"

	"kc/kernel"
	"kc/snapshot"
	"kc/snapshot/commandlog"
)

func TestReceiptAuthorizationUsesDurableRepositoryScope(t *testing.T) {
	dir := t.TempDir()
	ledger, err := commandlog.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = ledger.Execute("published-command", "digest", commandlog.Request{RepositoryID: "kr://owned/repo", TargetRef: string(snapshot.DefaultRef)}, func() (any, error) { return map[string]any{"newCommit": "published"}, nil })
	if err != nil {
		t.Fatal(err)
	}
	if err := WriteAllow(dir, AllowFile{Version: allowVersion, Rules: []AllowRule{{ID: "receipt", Principal: "kaiqidong", Repo: "kr://owned/repo", Ref: string(snapshot.DefaultRef), Actions: []string{"writer.receipt.read"}}}}); err != nil {
		t.Fatal(err)
	}
	cx := &invocation{Command: "writer-receipt", Home: dir, WS: &Home{Commands: ledger}, Flags: map[string]FlagValue{"as": "kaiqidong", "command-id": "published-command"}}
	if err := authorize(dir, "writer.receipt.read", authorizationFlags(cx), nil); err != nil {
		t.Fatalf("author cannot query own published operation: %v", err)
	}
	cx.Flags["repo"] = "kr://spoofed/repo"
	if actual := FlagString(authorizationFlags(cx), "repo"); actual != "kr://owned/repo" {
		t.Fatalf("receipt scope trusts request instead of durable command: %q", actual)
	}
	cx.Flags["as"] = "other-user"
	if err := authorize(dir, "writer.receipt.read", authorizationFlags(cx), nil); kernel.CodeOf(err) != kernel.ErrForbidden {
		t.Fatalf("other user read receipt: %v", err)
	}
	cx.Flags["as"] = "kaiqidong"
	if err := WriteAllow(dir, AllowFile{Version: allowVersion, Rules: []AllowRule{}}); err != nil {
		t.Fatal(err)
	}
	if err := authorize(dir, "writer.receipt.read", authorizationFlags(cx), nil); kernel.CodeOf(err) != kernel.ErrForbidden {
		t.Fatalf("revocation ignored: %v", err)
	}
}
