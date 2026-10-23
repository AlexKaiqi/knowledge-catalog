package systemrepo_test

import (
	"strings"
	"testing"

	"kc/systemrepo"
)

func retiredSchemaIndexClaim(body string) bool {
	for _, claim := range []string{
		"Schema 中的字段 access 是唯一书写位置",
		"字段 access 是唯一的索引声明书写位置",
		"Schema 的字段结构及其 access 声明支持的访问方式",
		"Schema 字段上的逻辑访问声明",
		"字段 access 同时声明允许的查询能力",
		"Schema 同时承载结构与字段访问意图",
		"读取其结构与字段访问声明",
	} {
		if strings.Contains(body, claim) {
			return true
		}
	}
	return false
}

func TestSystemPublicationDoesNotAssignIndexDeclarationsToSchemaFields(t *testing.T) {
	for _, op := range systemrepo.SystemPublicationOperations() {
		value, _ := op.Value.(map[string]any)
		body, _ := value["body"].(string)
		if retiredSchemaIndexClaim(body) {
			t.Errorf("%s still assigns retrieval declarations to structure-only Schema fields", op.PathHint)
		}
	}
}

func TestSchemaIndexPublicationGuardDistinguishesRetiredClaims(t *testing.T) {
	for _, body := range []string{
		"Schema 中的字段 access 是唯一书写位置。",
		"先了解 Schema 的字段结构及其 access 声明支持的访问方式。",
		"字段 access 同时声明允许的查询能力。",
		"Schema 同时承载结构与字段访问意图。",
		"精确读取其结构与字段访问声明。",
	} {
		if !retiredSchemaIndexClaim(body) {
			t.Fatalf("retired claim escaped: %s", body)
		}
	}
	for _, body := range []string{
		"Schema 不接受字段 access，索引声明是独立成员。",
		"先读 Schema 的字段结构，再读独立的索引声明。",
	} {
		if retiredSchemaIndexClaim(body) {
			t.Fatalf("structure-only guidance was rejected: %s", body)
		}
	}
}

func retiredSearchOrPluginClaim(body string) bool {
	for _, claim := range []string{
		"在明确仓或 Dataset 范围内",
		"历史 Schema 访问声明",
		"历史 Schema/ResourceDescriptor 声明",
		"插件按对象 Schema 生效",
		"插件按 Schema 声明对对象生效",
		"插件可以按 Schema 为对象增加访问能力",
	} {
		if strings.Contains(body, claim) {
			return true
		}
	}
	return false
}

func TestSystemPublicationDoesNotOfferRetiredSearchOrPluginAdoption(t *testing.T) {
	for _, op := range systemrepo.SystemPublicationOperations() {
		value, _ := op.Value.(map[string]any)
		body, _ := value["body"].(string)
		if retiredSearchOrPluginClaim(body) {
			t.Errorf("%s offers repository search or Schema plugin adoption instead of Dataset declarations", op.PathHint)
		}
	}
}

func TestSearchAndPluginPublicationGuardDistinguishesCurrentDeclarations(t *testing.T) {
	for _, body := range []string{
		"在明确仓或 Dataset 范围内寻找候选。",
		"插件按对象 Schema 生效。",
		"先核对 LiveAccess 实现或历史 Schema/ResourceDescriptor 声明。",
	} {
		if !retiredSearchOrPluginClaim(body) {
			t.Fatalf("retired claim escaped: %s", body)
		}
	}
	for _, body := range []string{
		"在 Dataset 的固定修订中搜索，按同一修订回读。",
		"插件按声明形状认领，Schema 只描述结构。",
		"外部取值声明在方面或成员上引用 ResourceDescriptor。",
	} {
		if retiredSearchOrPluginClaim(body) {
			t.Fatalf("current declaration guidance was rejected: %s", body)
		}
	}
}

func TestDatasetProjectionPublicationExplainsExplicitRecovery(t *testing.T) {
	for _, op := range systemrepo.SystemPublicationOperations() {
		if op.PathHint != "capabilities/projections/usage.md" {
			continue
		}
		value, _ := op.Value.(map[string]any)
		body, _ := value["body"].(string)
		// The recovery path is the explicit preparation operation, so the guard
		// names that operation rather than the retired sync spelling.
		if strings.Contains(body, "持续对账保证恢复") || !strings.Contains(body, "显式准备恢复") {
			t.Fatal("Dataset projection guidance promises unavailable automatic recovery")
		}
		return
	}
	t.Fatal("Dataset projection maintenance guidance is absent")
}
