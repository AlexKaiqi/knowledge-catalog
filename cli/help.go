package cli

import (
	"fmt"
	"sort"
	"strings"
)

// rootHelp is deliberately an index, not a command dump. Command and flag detail
// is disclosed by group and leaf help.
const rootHelp = `kc — Knowledge Catalog CLI

身份        登录并查看当前身份与已有权限
组合        查看登记空间、挂接 Repository、发权
知识        发现对象类型、搜索和读取知识
发布        把目录里的最终样子发布到一个 Repository
治理（进阶）校验并合入 Proposal
运维        投影、Hook、Gate 与审计
部署        初始化、恢复并运行 Server

最短旅程：kc help consume | kc help write | kc help compose
仓与文件交付：kc help composition | kc help dataset
授权与登记历史：kc help grant | kc help registry
只读文件视图：kcfs help
命令帮助：kc help <组> [<动词>]
`

const consumeHelp = `kc help consume — 连接服务并读取知识

  kc login
  kc show
  kc schema list --repo <id>
  kc read --repo <id> --object <object-id>
  kc search --repo <id> --query 'merchandise'

已定义 Dataset：
  kc search --dataset <id> --query 'merchandise'
  kc read --dataset <id> --object <object-id>
回执带 repository / objectId / commit。不要先造一份 pin 文件。
`

const writeHelp = `kc help write — 创建并发布一个 Repository

  kc create --name <名称>
  # 或：kc create --url <repository-url> --credential-file <path>
  kc writer commit --command-id <id> --repo <id> --dir <drafts>
  kc attach --repo <id>
  kc grant add --repo <id> --principal <user> \
    --action knowledge.read,knowledge.search,knowledge.schema.read

提交前可看：kc diff --repo <id> --dir <drafts>
`

const composeHelp = `kc help compose — 组合 Repository 并发权

  kc show
  kc attach --repo <id>
  kc dataset define <id> --revision <n> --source <repo>[=<selector>][@path[@subPath]]
  # 逐文件交付：--file-source <repo>@<来源文件>@<交付路径>
  kc grant add --dataset <id> --principal <user> \
    --action dataset.resolve,file.read
  kc grant add --repo <id> --principal <user> --action <actions>
  kc dataset clone <id> <dir>   # 把交付目录树物化成本地目录
`

type helpEntry struct {
	Description string
	Usage       string
}

var helpEntries = mergeHelpEntries(snapshotHelpEntries(), map[string]helpEntry{
	"login": {
		Description: "验证身份并保存当前 Server 的本机会话",
		Usage: `kc login [--mode taihu|token|local] [--wait]
  Server 来自 --server、KC_SERVER_URL 或 client.json；只有 local 模式使用 --as。
  例：kc login --mode local --as kaiqidong`,
	},
	"logout": {
		Description: "清除当前 Server 的本机会话",
		Usage:       `kc logout`,
	},
	"whoami": {
		Description: "显示当前认证主体",
		Usage:       `kc whoami`,
	},
	"admission show": {
		Description: "显示本人已有权限和外部申请入口",
		Usage:       `kc admission show`,
	},
	"deployment init": {
		Description: "初始化部署",
		Usage: `kc deployment init --config <deployment.yaml>
  --config 是部署文件路径，不是登记空间标识。
  配置必须绑定实际 system 仓；初始化发布系统知识，启动只核对。
  例：kc deployment init --config deployment.yaml`,
	},
	"deployment status": {
		Description: "查看部署恢复状态",
		Usage: `kc deployment status --config <deployment.yaml>
  例：kc deployment status --config deployment.yaml`,
	},
	"deployment system publish": {
		Description: "把系统知识出版物写入绑定仓",
		Usage: `kc deployment system publish --config <deployment.yaml>
  例：kc deployment system publish --config deployment.yaml`,
	},
	"deployment identity migrate": {
		Description: "迁移历史身份绑定",
		Usage: `kc deployment identity migrate --config <deployment.yaml> --file <request.json>
  --file 是身份迁移请求 JSON。
  例：kc deployment identity migrate --config deployment.yaml --file migrate.json`,
	},
	"governance proposal create": {
		Description: "把变更写到 candidate 并打开 Proposal，不推进 target",
		Usage: `kc governance proposal create --repo <id> --proposal-id <id> --candidate <ref> (--value <json> | --file <path> | --changeset <file>) [--object <id>] [--target <ref>] [--base <commit>] [--message <text>]
  把这次变更写到 --candidate，不推进 --target（默认 published）。--object/--value 写法和 writer put 相同。
  例：kc governance proposal create --repo kr://scene/knowledge --proposal-id PR-1 --candidate refs/heads/change --object note/hello --value '{"text":"proposed"}'`,
	},
	"governance proposal merge": {
		Description: "合入 Proposal",
		Usage:       `kc governance proposal merge --proposal <id>`,
	},
	"governance preview create": {
		Description: "把 proposal 叠到本次解析的 Dataset 上，给出 candidate 坐标，不改 main",
		Usage: `kc governance preview create --proposal <id> --dataset <id>
  本次命令解析 Dataset 一次，冻进 Preview。stdout 是 Preview 坐标（previewId、candidate），不是合入后正文。
  例：kc governance preview create --proposal PR-1 --dataset scene-notes`,
	},
	"governance preview validate": {
		Description: "校验 Preview 的协议结构",
		Usage:       `kc governance preview validate --preview <id>`,
	},
	"governance validation record": {
		Description: "记录外部套件结果",
		Usage: `kc governance validation record --preview <id> --outcome <PASSED|FAILED> --suite <id> [--message <text>]
  --outcome 只接受 PASSED 或 FAILED，是调用方断言，不在这里跑套件。
  例：kc governance validation record --preview PV-1 --outcome PASSED --suite structure`,
	},
	"operations projection describe": {
		Description: "查看投影状态",
		Usage:       `kc operations projection describe --repo <id> [--commit <id>]`,
	},
	"operations projection sync": {
		Description: "同步历史或排障投影",
		Usage:       `kc operations projection sync --repo <id> [--commit <id>]`,
	},
	"operations projection notice": {
		Description: "通知 Bound State 观察变化，不推进仓 HEAD",
		Usage: `kc operations projection notice --repo <id>
  通知 Bound State 观察变化，不推进仓 HEAD。指定句柄时再加 --object 与 --aspect。`,
	},
	"operations access-spec describe": {
		Description: "查看 AccessSpec",
		Usage:       `kc operations access-spec describe (--repo <id> | --dataset <id>)`,
	},
	"operations hook add": {
		Description: "登记 Hook",
		Usage: `kc operations hook add --on <verb> --phase <phase> [--repo <id>] [--url <url>] [--run <command>]
  --on 是会改状态的语义动作（如 writer.commit）。--phase 只接受 pre 或 post。
  --run 与 --url 二选一；pre 必须 --run。
  例：kc operations hook add --on writer.commit --phase pre --repo kr://scene/knowledge --run check.sh`,
	},
	"operations hook list": {
		Description: "列出 Hook",
		Usage:       `kc operations hook list [--on <verb>] [--repo <id>]`,
	},
	"operations hook remove": {
		Description: "移除 Hook",
		Usage:       `kc operations hook remove --id <id>`,
	},
	"operations gate add": {
		Description: "登记 Gate",
		Usage: `kc operations gate add --on <event> --require <evidence> [--repo <id>]
  --on 目前只接受 merge。--require 是 validate 或 suite:<name>，逗号分隔。merge 必须带 --repo。
  例：kc operations gate add --on merge --require validate,suite:lint --repo kr://scene/knowledge`,
	},
	"operations gate list": {
		Description: "列出 Gate",
		Usage:       `kc operations gate list [--on <event>] [--repo <id>]`,
	},
	"operations gate remove": {
		Description: "移除 Gate",
		Usage:       `kc operations gate remove --id <id>`,
	},
	"operations audit access": {
		Description: "查看访问账",
		Usage:       `kc operations audit access [--repo <id>] [--object <id>] [--limit <n>] [--continuation <cursor>]`,
	},
	"operations audit trace": {
		Description: "查看请求追踪",
		Usage:       `kc operations audit trace --trace-id <id>`,
	},
	"operations audit hitmap": {
		Description: "按对象汇总检索命中，不是访问事件账",
		Usage: `kc operations audit hitmap [--limit <n>] [--continuation <cursor>]
  按对象汇总命中。source 是 hitmap，不是 access。`,
	},
	"operations feedback record": {
		Description: "记录检索反馈",
		Usage: `kc operations feedback record --trace-id <id> --dataset <id> --outcome <answered|accepted|rejected|corrected|helpful|unhelpful> [--message <text>]
  --outcome 只能是 enumerated 那六个词，不是 PASSED/FAILED。
  例：kc operations feedback record --trace-id tr-1 --dataset scene-set --outcome helpful`,
	},
	"schema list": {
		Description: "列出一个 Repository 已发布的对象类型，不是对象实例目录或合同正文",
		Usage:       `kc schema list --repo <id> [--limit <n>] [--continuation <cursor>]`,
	},
	"search": {
		Description: "在固定仓或 Dataset 上按 Schema 字段的查询声明检索知识，列出匹配对象",
		Usage: `kc search (--repo <id> | --dataset <id>) --query <text>
  --query 是 MATCH 关键字，不是文件 glob，也不是「搜 * 等于浏览」。可过滤字段先 schema list / schema describe。
  精确条件用 --eq path=value；路径有歧义时写成 schema::aspect::path。
  召回策略用 --recall lexical|semantic（省略即 lexical；semantic 需要向量投影与 embedding 能力，缺则失败关闭）。
  含空格、* 或 $ 的值加单引号，否则 shell 会展开。
  例：kc search --repo kr://scene/knowledge --query 'merchandise'
  例：kc search --dataset scene-set --query 'merchandise'
  例：kc search --repo kr://scene/knowledge --eq unit=CNY
  例：kc search --repo kr://scene/knowledge --eq 'name=Gross merchandise value'`,
	},
	"read": {
		Description: "读取一个知识对象在固定 commit 上的正文",
		Usage: `kc read (--repo <id> | --dataset <id>) --object <object-id>
  --object 是知识对象 id，不是文件路径。读 README 加 --aspect readme。回执带 commit。
  例：kc read --repo kr://scene/knowledge --object metric/gmv
  例：kc read --dataset scene-set --object metric/gmv
  例：kc read --repo kr://kc/system --object kc/concept/object --aspect definition`,
	},
	"serve": {
		Description: "启动 typed API Server；浏览器打开根路径会 404",
		Usage: `kc serve --config deployment.yaml [--listen <address>]
  这是 typed API，浏览器打开根路径会 404，不是敲 kc 的页面。
  例：kc serve --config deployment.yaml --listen 127.0.0.1:7380`,
	},
	"resolve": {
		Description: "检查对象是否存在并给出解析到的 commit，不打开正文",
		Usage: `kc resolve (--repo <id> | --dataset <id>) --object <object-id>
  --object 同 read。成功给出 commit，不打开正文。缺对象是 status=UNRESOLVED（单仓）或空名单（Dataset）。
  例：kc resolve --repo kr://scene/knowledge --object metric/gmv
  例：kc resolve --dataset scene-set --object metric/gmv`,
	},
	"relations": {
		Description: "列出连着对象的关系对象命中，不是邻接对象正文",
		Usage: `kc relations (--repo <id> | --dataset <id>) --object <object-id> [--direction DIRECTED|UNDIRECTED] [--limit <n>] [--continuation <cursor>]
  --object 同 read。hits 的 objectId 是命中的关系对象坐标；邻居对象只在关系端点里，不在 hits 顶层。
  --direction 是关系边的方向类型，不是 in/out 遍历。需要检索投影。邻接遍历走 traverse。
  例：kc relations --repo kr://scene/knowledge --object metric/gmv --direction DIRECTED
  例：kc relations --dataset scene-set --object metric/gmv --direction DIRECTED`,
	},
	"traverse": {
		Description: "在固定范围内沿关系做有界邻域遍历，不是图查询语言",
		Usage: `kc traverse (--repo <id> | --dataset <id>) --object <object-id> --max-hops <1-8> [--min-hops <n>] [--relation-type <t>] [--role <r>] [--direction DIRECTED|UNDIRECTED] [--limit <n>] [--continuation <cursor>]
  范围在请求开始固定：Dataset 是清单成员 pin，单仓是该仓 commit。nodes 是按对象去重的到达对象（含最小跳深），
  edges 是入选关系（保留关系对象坐标与完整端点）；引用到范围外对象的 frontier 止步并列入 boundary。
  不枚举全部路径，不承诺最短路径；游标续传闭包增量。
  例：kc traverse --dataset scene-set --object kc://acme/public/core/Table:orders --max-hops 3
  例：kc traverse --repo kr://scene/knowledge --object Table:orders --max-hops 2 --relation-type owned-by`,
	},
	"provenance": {
		Description: "读取对象来源信封",
		Usage: `kc provenance (--repo <id> | --dataset <id>) --object <object-id>
  --object 同 read。
  例：kc provenance --repo kr://scene/knowledge --object metric/gmv
  例：kc provenance --dataset scene-set --object metric/gmv`,
	},
	"log": {
		Description: "分页读取对象修订历史",
		Usage: `kc log (--repo <id> | --dataset <id>) --object <object-id> [--limit <n>] [--continuation <cursor>]
  --object 同 read。这是对象修订史，不要加 --aspect。
  例：kc log --repo kr://scene/knowledge --object metric/gmv
  例：kc log --dataset scene-set --object metric/gmv`,
	},
	"schema describe": {
		Description: "列出字段能否 filter、text、sort，以及 Bound State 访问原点",
		Usage: `kc schema describe --repo <id> --object <schema-id>
  --object 是 schema/* 合同 id，来自 schema list 的 objectId。打印字段 AccessHints 与可选 origin（frontmatter 访问路径）。
  例：kc schema describe --repo kr://kc/system --object schema/meta/schema-definition/v1`,
	},
	"binding show": {
		Description: "查看 Binding 声明",
		Usage: `kc binding show (--repo <id> | --dataset <id>) --object <object-id> --aspect <name>
  查看该对象 Aspect 的 Bound State 声明（Schema origin），不读墙外值。
  例：kc binding show --repo kr://scene/knowledge --object table/orders --aspect stats
  例：kc binding show --dataset scene-set --object table/orders --aspect stats`,
	},
	"access": {
		Description: "按 Schema origin 与对象 ID 读取该 Aspect 的当前观察",
		Usage: `kc access (--repo <id> | --dataset <id>) --object <object-id> --aspect <name>
  访问路径在 Domain Schema frontmatter 的 origin。协议坐标是 origin + 对象 object_id，返回该 Aspect。
  不要加 --operation（那是 invoke）。
  例：kc access --repo kr://scene/knowledge --object table/orders --aspect stats
  例：kc access --dataset scene-set --object table/orders --aspect stats`,
	},
	"invoke": {
		Description: "调用对象声明的 Interface 能力或兼容的 ResourceDescriptor 操作",
		Usage: `kc invoke (--repo <id> | --dataset <id>) --object <object-id> --implementation <member-key> --operation <name> --input <json>
  --implementation 选择 interfaces 切面中的一个实现；合同、实现 Schema 和处理器按固定版本匹配。
  --input 是一个 JSON 值，用单引号包住。LiveAccess 的 read 返回当前值及独立观察依据，invoke 调用允许的源操作。
  例：kc invoke --repo kr://scene/knowledge --object table/orders --implementation live --operation read --input '{}'
  省略 --implementation 时兼容旧 ResourceDescriptor：
  例：kc invoke --repo kr://scene/knowledge --object handle/orders --operation preview --input '{}'
  例：kc invoke --dataset scene-set --object handle/orders --operation preview --input '{}'`,
	},
	"diff": {
		Description: "对照当前发布版本，列出这个目录会改哪些对象",
		Usage: `kc diff --repo <id> --dir <drafts>
  --dir 是期望正文。对照当前发布版本列出 add / update，不写仓。
  例：kc diff --repo kr://scene/knowledge --dir drafts`,
	},
	"writer commit": {
		Description: "把目录里的最终样子发布成仓的新版本",
		Usage: `kc writer commit --command-id <id> --repo <id> --dir <drafts> [--bulk]
  --dir 是期望正文。Client 对照当前版本求差后提交。--command-id 由你本次提供，Client 不代填。
  --bulk 一次性接入大量文件时走介质批量通路；介质不支持时整条拒绝，不带时行为不变。
  例：kc writer commit --command-id commit-1 --repo kr://scene/knowledge --dir drafts`,
	},
	"writer put": {
		Description: "直接发布一个 Address，不经过草稿目录",
		Usage: `kc writer put --command-id <id> --repo <id> --object <object-id> (--value <json> | --file <path>)
  直接发布一个 Address，不经过草稿目录。--value 是 JSON 对象，用单引号包住。
  例：kc writer put --command-id put-1 --repo kr://scene/knowledge --object metric/gmv --value '{"name":"GMV"}'`,
	},
	"writer remove": {
		Description: "移除一个 Address",
		Usage: `kc writer remove --command-id <id> --repo <id> --object <object-id>
  --object 同 read。--command-id 由你本次提供。
  例：kc writer remove --command-id rm-1 --repo kr://scene/knowledge --object metric/gmv`,
	},
	"writer head": {
		Description: "查看 Repository 当前发布版本",
		Usage:       `kc writer head --repo <id>`,
	},
	"writer receipt": {
		Description: "查询 Writer 命令回执",
		Usage:       `kc writer receipt --command-id <id>`,
	},
})

func mergeHelpEntries(base, extra map[string]helpEntry) map[string]helpEntry {
	entries := make(map[string]helpEntry, len(base)+len(extra))
	for _, group := range []map[string]helpEntry{base, extra} {
		for path, entry := range group {
			if _, exists := entries[path]; exists {
				panic(fmt.Sprintf("duplicate CLI help entry %q", path))
			}
			entries[path] = entry
		}
	}
	return entries
}

// complexLeafOperands is the closed set of leaves whose values have a writing
// rule. Identity-only commands stay a one-line skeleton. Tested.
var complexLeafOperands = []string{
	"login", "create", "grant add", "grant list",
	"dataset define", "dataset overlay", "search", "read", "resolve", "relations", "traverse",
	"provenance", "log", "schema describe", "binding show", "access", "invoke",
	"diff", "writer commit", "writer put", "writer remove",
	"governance proposal create", "governance preview create",
	"governance validation record", "operations hook add", "operations gate add",
	"operations feedback record", "deployment init", "deployment status",
	"deployment system publish", "deployment identity migrate", "serve",
}

var helpGroups = map[string][]string{
	"identity":    {"login", "logout", "whoami", "admission show"},
	"admission":   {"admission show"},
	"composition": {"show", "create", "attach", "detach", "registry audit", "grant add", "grant list", "grant remove", "dataset define", "dataset clone", "dataset retire", "dataset overlay"},
	"registry":    {"registry audit"},
	"dataset":     {"dataset define", "dataset clone", "dataset retire", "dataset overlay", "show", "grant add", "grant list", "grant remove"},
	"grant":       {"grant add", "grant list", "grant remove"},
	"knowledge":   {"schema list", "search", "read", "resolve", "relations", "traverse", "provenance", "log", "schema describe", "binding show", "access", "invoke"},
	"publishing":  {"diff", "writer commit", "writer put", "writer remove", "writer head", "writer receipt"},
	"writer":      {"writer commit", "writer put", "writer remove", "writer head", "writer receipt"},
	"governance":  {"governance proposal create", "governance preview create", "governance preview validate", "governance validation record", "governance proposal merge"},
	"operations":  {"operations projection describe", "operations projection sync", "operations projection notice", "operations access-spec describe", "operations hook add", "operations hook list", "operations hook remove", "operations gate add", "operations gate list", "operations gate remove", "operations audit access", "operations audit trace", "operations audit hitmap", "operations feedback record"},
	"deployment":  {"deployment init", "deployment status", "deployment system publish", "deployment identity migrate", "serve"},
}

func formatLeafHelp(path string) string {
	entry, ok := helpEntries[path]
	if !ok {
		return ""
	}
	if entry.Description == "" {
		return entry.Usage
	}
	return entry.Description + "\n\n" + entry.Usage
}

func usageForFailure(path string) string {
	path = strings.TrimSpace(path)
	if path == "" || path == "help" {
		return "kc help"
	}
	if help := formatLeafHelp(path); help != "" {
		return help
	}
	if _, ok := cliSurface[path]; ok {
		return "kc " + path
	}
	if i := strings.LastIndex(path, " "); i > 0 {
		if help := formatLeafHelp(path[:i]); help != "" {
			return help
		}
		if text, err := helpFor(path[:i]); err == nil {
			return strings.TrimSpace(text)
		}
	}
	return "kc help"
}

func helpFor(topic string) (string, error) {
	topic = strings.ToLower(strings.TrimSpace(topic))
	switch topic {
	case "":
		return rootHelp, nil
	case "consume":
		return consumeHelp, nil
	case "write":
		return writeHelp, nil
	case "compose":
		return composeHelp, nil
	}
	if help := formatLeafHelp(topic); help != "" {
		return help + "\n", nil
	}
	if paths, ok := helpGroups[topic]; ok {
		help := formatGroupHelp(topic, paths)
		if topic == "dataset" {
			help += "\n只读文件视图：kcfs help\n"
		}
		return help, nil
	}
	paths := make([]string, 0)
	for path := range cliSurface {
		if path == topic || strings.HasPrefix(path, topic+" ") {
			paths = append(paths, path)
		}
	}
	if len(paths) > 0 {
		sort.Strings(paths)
		return formatGroupHelp(topic, paths), nil
	}
	return "", fmt.Errorf("unknown help topic %s; want consume, write, compose, or a command group", topic)
}

func formatGroupHelp(topic string, paths []string) string {
	lines := []string{"kc help " + topic, ""}
	for _, path := range paths {
		description := helpEntries[path].Description
		if description == "" {
			description = "进阶命令；运行叶子 help 查看操作数"
		}
		lines = append(lines, fmt.Sprintf("  kc %-42s %s", path, description))
	}
	return strings.Join(lines, "\n") + "\n"
}

func leafNeedsValueHelp(usage string) bool {
	first, _, _ := strings.Cut(usage, "\n")
	for _, mark := range []string{
		"<json>", "<text>", "<action", "[=<", "--mode", "--config",
		"--changeset", "--value", "--input", "--query", "--source",
		"--phase", "--require", "--candidate", "--outcome", "--dir ",
		"--operation", "--credential-file", "--file", "--aspect",
		"--direction", "--suite",
	} {
		if strings.Contains(first, mark) {
			return true
		}
	}
	return false
}
