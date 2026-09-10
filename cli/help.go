package cli

import (
	"fmt"
	"sort"
	"strings"
)

// Help is deliberately an index, not a command dump. Command and flag detail
// is disclosed by group and leaf help.
const Help = `kc — Knowledge Catalog CLI

身份        登录并查看当前身份与已有权限
组合        选择 Catalog、查看和挂接 Repository、发权
知识        浏览 Schema、搜索和读取知识
发布        预处理草稿并发布到一个 Repository
治理（进阶）校验并合入 Proposal
运维        投影、Hook、Gate 与审计
部署        初始化、恢复并运行 Server

最短旅程：kc help consume | kc help write | kc help compose
命令帮助：kc help <组> [<动词>]
`

const ConsumeHelp = `kc help consume — 进入 Catalog 并读取知识

  kc login
  kc catalog list
  kc catalog use <id>                 # 只有一间可见时可省
  kc show
  kc knowledge schema list --repo <id>
  kc knowledge read --repo <id> --object <object-id>
  kc knowledge search --repo <id> --query <text>

仅当同一任务需要两个以上 Repository：
  kc workspace pin --source <repo> --source <repo> --out pin.json
  kc knowledge search --pin pin.json --query <text>
`

const WriteHelp = `kc help write — 创建并发布一个 Repository

  kc create --name <名称>
  # 或：kc create --url <repository-url> --credential-file <path>
  kc pack --repo <id> --dir <drafts> --out changeset.json
  kc writer commit --command-id <id> --changeset changeset.json
  kc attach --repo <id>
  kc grant add --repo <id> --principal <user> \
    --action knowledge.read,knowledge.search,knowledge.schema.read
`

const ComposeHelp = `kc help compose — 组合 Repository 并发权

  kc catalog list
  kc catalog use <id>
  kc show
  kc attach --repo <id>
  kc workspace define <id> --revision <n> --source <repo>
  kc grant add --repo <id> --principal <user> --action <actions>
`

var helpDescriptions = map[string]string{
	"login":                     "验证身份并保存当前 Server 的本机会话",
	"logout":                    "清除当前 Server 的本机会话",
	"whoami":                    "显示当前认证主体",
	"admission show":            "显示本人已有权限和外部申请入口",
	"catalog list":              "列出可见 Catalog",
	"catalog use":               "选择当前 Catalog",
	"show":                      "显示当前 Catalog 的 Repository 与知识集",
	"create":                    "创建托管 Repository 或连接自有 Repository",
	"attach":                    "把已连接 Repository 挂入当前 Catalog",
	"detach":                    "从当前 Catalog 拿下 Repository",
	"grant add":                 "授予 Repository 或 Catalog 范围动作",
	"grant list":                "列出授权规则",
	"grant remove":              "撤销一条授权规则",
	"workspace pin":             "固定一次多 Repository 任务",
	"workspace define":          "定义可复用的多 Repository 配方",
	"workspace retire":          "退役命名配方",
	"workspace check":           "检查 pin 的成员仍然可用",
	"workspace overlay":         "在本机合并 Workspace overlay",
	"knowledge schema list":     "分页浏览一个 Repository 的 Schema",
	"knowledge search":          "在一个 Repository 或 pin 中搜索",
	"knowledge read":            "读取一个知识对象",
	"knowledge resolve":         "检查对象是否存在",
	"knowledge relations":       "读取对象的一跳关系",
	"knowledge provenance":      "读取对象来源信封",
	"knowledge log":             "分页读取对象修订历史",
	"knowledge schema describe": "查看 Schema 字段访问语义",
	"knowledge binding show":    "查看 Binding 声明",
	"knowledge access":          "通过 Binding 读取墙外观察",
	"knowledge invoke":          "调用 ResourceDescriptor 操作",
	"pack":                      "把本机草稿预处理为 ChangeSet",
	"writer commit":             "提交一个 ChangeSet",
	"writer put":                "发布一个 Address",
	"writer remove":             "移除一个 Address",
	"writer head":               "查看 Repository 当前发布版本",
	"writer receipt":            "查询 Writer 命令回执",
}

var leafUsage = map[string]string{
	"login": `kc login [--mode taihu|token|local] [--wait]
  Server 来自 --server、KC_SERVER_URL 或 client.json；只有 local 模式使用 --as。`,
	"catalog use":           `kc catalog use <id>`,
	"show":                  `kc show`,
	"create":                `kc create (--name <名称> [--store <store>] | --url <url> --credential-file <path>)`,
	"attach":                `kc attach --repo <id>`,
	"detach":                `kc detach --repo <id>`,
	"grant add":             `kc grant add --principal <id> --action <action[,action...]> (--repo <id> | --catalog <id>)`,
	"grant remove":          `kc grant remove --id <rule-id>`,
	"knowledge schema list": `kc knowledge schema list --repo <id> [--limit <n>] [--continuation <cursor>]`,
	"knowledge search":      `kc knowledge search (--repo <id> | --pin <file>) --query <text>`,
	"knowledge read":        `kc knowledge read (--repo <id> | --pin <file>) --object <object-id>`,
	"writer commit": `kc writer commit --command-id <id> --changeset <file>
  ChangeSet 内含 Repository；同时给 --repo 时必须一致。`,
	"writer put":                `kc writer put --command-id <id> --repo <id> --object <object-id> (--value <json> | --file <path>)`,
	"writer remove":             `kc writer remove --command-id <id> --repo <id> --object <object-id>`,
	"workspace pin":             `kc workspace pin (--source <repo>[=<selector>]... | --workspace <id>) [--out <pin.json>]`,
	"governance preview create": `kc governance preview create --proposal <id> --pin <pin.json>`,
	"serve":                     `kc serve --config deployment.yaml [--listen <address>]`,
}

var helpGroups = map[string][]string{
	"identity":    {"login", "logout", "whoami", "admission show"},
	"composition": {"catalog list", "catalog use", "show", "create", "attach", "detach", "grant add", "grant list", "grant remove", "workspace pin", "workspace define", "workspace retire", "workspace check", "workspace overlay"},
	"catalog":     {"catalog list", "catalog use", "catalog audit", "catalog archive"},
	"workspace":   {"workspace pin", "workspace check", "workspace define", "workspace retire", "workspace overlay"},
	"grant":       {"grant add", "grant list", "grant remove"},
	"knowledge":   {"knowledge schema list", "knowledge search", "knowledge read", "knowledge resolve", "knowledge relations", "knowledge provenance", "knowledge log", "knowledge schema describe", "knowledge binding show", "knowledge access", "knowledge invoke"},
	"publishing":  {"pack", "writer commit", "writer put", "writer remove", "writer head", "writer receipt"},
	"writer":      {"writer commit", "writer put", "writer remove", "writer head", "writer receipt"},
	"governance":  {"governance proposal create", "governance preview create", "governance preview validate", "governance validation record", "governance proposal merge"},
	"operations":  {"operations projection describe", "operations projection sync", "operations projection notice", "operations access-spec describe", "operations hook add", "operations hook list", "operations hook remove", "operations gate add", "operations gate list", "operations gate remove", "operations audit access", "operations audit trace", "operations audit hitmap", "operations feedback record"},
	"deployment":  {"deployment init", "deployment status", "deployment system publish", "deployment identity migrate", "serve"},
}

func helpFor(topic string) (string, error) {
	topic = strings.ToLower(strings.TrimSpace(topic))
	switch topic {
	case "":
		return Help, nil
	case "consume":
		return ConsumeHelp, nil
	case "write":
		return WriteHelp, nil
	case "compose":
		return ComposeHelp, nil
	}
	if usage, ok := leafUsage[topic]; ok {
		return usage + "\n", nil
	}
	if paths, ok := helpGroups[topic]; ok {
		lines := []string{"kc help " + topic, ""}
		for _, path := range paths {
			description := helpDescriptions[path]
			if description == "" {
				description = "进阶命令；运行叶子 help 查看操作数"
			}
			lines = append(lines, fmt.Sprintf("  kc %-42s %s", path, description))
		}
		return strings.Join(lines, "\n") + "\n", nil
	}
	paths := make([]string, 0)
	for path := range cliSurface {
		if path == topic || strings.HasPrefix(path, topic+" ") {
			paths = append(paths, path)
		}
	}
	if len(paths) > 0 {
		sort.Strings(paths)
		lines := []string{"kc help " + topic, ""}
		for _, path := range paths {
			lines = append(lines, "  kc "+path)
		}
		return strings.Join(lines, "\n") + "\n", nil
	}
	return "", fmt.Errorf("unknown help topic %s; want consume, write, compose, or a command group", topic)
}
