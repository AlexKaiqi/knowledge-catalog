package cli_test

import (
	"maps"
	"slices"
	"strings"
	"testing"

	snapshotcli "kc/snapshot/cli"
)

func TestSnapshotHelpTopicsHaveStaticHelpNamespace(t *testing.T) {
	topics := snapshotcli.HelpTopics()
	var got []string
	for topic, text := range topics {
		got = append(got, topic)
		if strings.TrimSpace(text) == "" {
			t.Errorf("topic %s has no teaching content", topic)
		}
	}
	slices.Sort(got)
	want := []string{"commit", "composition", "dataset", "files", "grant", "metadata", "pin", "recipe", "ref", "registry", "repository", "selector", "snapshot", "versions"}
	if !slices.Equal(got, want) {
		t.Fatalf("static concept/group namespace changed:\n got: %v\nwant: %v", got, want)
	}
	for _, command := range snapshotcli.Commands() {
		if _, collision := topics[command.Path]; collision {
			t.Errorf("help topic %s became a business leaf", command.Path)
		}
		for _, group := range command.Groups {
			if _, explained := topics[group]; !explained {
				t.Errorf("module leaf %s exposes unexplained module group %s", command.Path, group)
			}
		}
	}
	for _, host := range []string{"consume", "compose", "write"} {
		if _, copied := topics[host]; copied {
			t.Errorf("module claimed host journey %s", host)
		}
	}
	for _, alias := range []string{"selector", "ref", "commit"} {
		if topics[alias] != topics["versions"] {
			t.Errorf("%s must refer to the same version explanation", alias)
		}
	}
}

func TestSnapshotHelpTopicsReturnsIndependentContribution(t *testing.T) {
	first := snapshotcli.HelpTopics()
	want := maps.Clone(first)
	for topic := range first {
		first[topic] = "caller mutation"
	}
	delete(first, "snapshot")
	first["caller-topic"] = "caller addition"
	if second := snapshotcli.HelpTopics(); !maps.Equal(second, want) {
		t.Fatalf("caller changed shared module teaching content:\n got: %v\nwant: %v", second, want)
	}
}

func TestSnapshotHelpTopicsExplainResourceVersionAndAuthorityBoundaries(t *testing.T) {
	// These are distinct public-contract facts, rather than headings or a
	// minimum length: a generic command list cannot satisfy each topic.
	claims := map[string][]string{
		"snapshot":    {"Repository", "commit", "Dataset revision", "不是新的可写 Repository", "来源仓后来提交，不改变", "当前授权", "Schema"},
		"repository":  {"独立提交历史", "写入边界", "create 不自动 attach", "不隐含授予消费权", "files list 只浏览已发布 Dataset", "原 lakeFS"},
		"versions":    {"ref", "commit", "Dataset revision", "refs/heads/main", "发布时", "baseRev", "commit ID 直接当作 selector", "固定版本不冻结权限"},
		"pin":         {"pinId", "Dataset revision", "commit", "不需要先创建 pin 文件", "不接收 --pin", "不授予权限", "当前撤权"},
		"recipe":      {"YAML", "不是已接受的 Dataset 发布", "mounts", "files", "metadata", "显式写 selector", "overlay", "不发布", "--out", "覆盖"},
		"metadata":    {"可选", "字符串键值", "原生元数据", "不随 commit 冻结", "repository.metadata.read", "repository.metadata.write", "{} 清空", "随 revision 固定", "不会自动继承"},
		"composition": {"Repository", "Dataset", "revision", "独立步骤", "授权", "files list/read"},
		"dataset":     {"只选一个仓", "多个仓", "不是新的可写 Repository", "旧版不可覆盖", "来源随后提交不改变", "当前权限", "退役"},
		"files":       {"--path 是交付树", "直接子项", "revision/path", "dataset.resolve", "file.read", "base64", "目标必须不存在", "clone 中途失败可留部分树"},
		"grant":       {"主体", "动作", "资源范围", "当前权限", "其它规则也可能继续放行", "规则说明和当前权限不是同一份保证", "不能追回"},
		"registry":    {"每套服务只有一个", "登记不会", "最近一段", "登记历史不是来源正文提交历史", "不提供历史续页", "保留来源文件/历史"},
	}
	topics := snapshotcli.HelpTopics()
	for topic, facts := range claims {
		t.Run(topic, func(t *testing.T) {
			text, present := topics[topic]
			if !present {
				t.Fatalf("missing teaching topic %s", topic)
			}
			for _, fact := range facts {
				if !strings.Contains(text, fact) {
					t.Errorf("%s omits the resource/version/authority fact %q", topic, fact)
				}
			}
		})
	}
}
