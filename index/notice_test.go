package index

import (
	"testing"

	"kc/kernel"
	"kc/knowledge"
)

func TestChangeNoticeAcceptsRepoRefOnly(t *testing.T) {
	notice, err := ParseChangeNotice([]byte(`{"repository":"kr://acme/public/core","ref":"refs/heads/main"}`))
	if err != nil {
		t.Fatal(err)
	}
	if notice.Repository != "kr://acme/public/core" || notice.refOrDefault() != "refs/heads/main" {
		t.Fatalf("notice = %#v", notice)
	}
}

func TestChangeNoticeAcceptsOptionalAddressAndRevisionHint(t *testing.T) {
	notice, err := ParseChangeNotice([]byte(`{
		"repository":"kr://acme/public/core",
		"address":{"kind":"Aspect","objectId":"Job:orders","aspectName":"runtime"},
		"sourceRevision":"r2"
	}`))
	if err != nil || notice.Address == nil || notice.Address.ObjectID != "Job:orders" || notice.SourceRevision != "r2" {
		t.Fatalf("notice = %#v %v", notice, err)
	}
}

func TestChangeNoticeRejectsBody(t *testing.T) {
	for _, raw := range []string{
		`{"repository":"kr://acme/public/core","value":{"status":"running"}}`,
		`{"repository":"kr://acme/public/core","body":"secret"}`,
		`{"repository":"kr://acme/public/core","payload":{}}`,
		`{"repository":"kr://acme/public/core","observations":[]}`,
	} {
		_, err := ParseChangeNotice([]byte(raw))
		if kernel.CodeOf(err) != kernel.ErrUsageInvalid {
			t.Fatalf("payload %s: %v", raw, err)
		}
	}
}

func TestChangeNoticeRequiresRepository(t *testing.T) {
	_, err := ParseChangeNotice([]byte(`{"ref":"refs/heads/main"}`))
	if kernel.CodeOf(err) != kernel.ErrUsageInvalid {
		t.Fatalf("err=%v", err)
	}
	if err := ValidateChangeNotice(ChangeNotice{Address: &knowledge.Address{Kind: knowledge.KindAspect}}); kernel.CodeOf(err) != kernel.ErrUsageInvalid {
		t.Fatalf("empty address objectId: %v", err)
	}
}
