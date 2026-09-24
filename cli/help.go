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
知识        列出实体、搜索和读取知识
发布        把目录里的最终样子发布到一个 Repository
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
  kc schema list --repo <id>
  kc read --repo <id> --object <object-id>
  kc search --repo <id> --query 'merchandise'

已定义 Dataset：
  kc search --dataset <id> --query 'merchandise'
  kc read --dataset <id> --object <object-id>
回执带 repository / objectId / commit。不要先造一份 pin 文件。
`

const WriteHelp = `kc help write — 创建并发布一个 Repository

  kc create --name <名称>
  # 或：kc create --url <repository-url> --credential-file <path>
  kc writer commit --command-id <id> --repo <id> --dir <drafts>
  kc attach --repo <id>
  kc grant add --repo <id> --principal <user> \
    --action knowledge.read,knowledge.search,knowledge.schema.read

提交前可看：kc diff --repo <id> --dir <drafts>
`

const ComposeHelp = `kc help compose — 组合 Repository 并发权

  kc catalog list
  kc catalog use <id>
  kc show
  kc attach --repo <id>
  kc dataset define <id> --revision <n> --source <repo>[=<selector>][@path[@subPath]]
  # 逐文件交付：--file-source <repo>@<来源文件>@<交付路径>
  kc grant add --catalog <id> --dataset <id> --principal <user> \
    --action file.read
  kc grant add --repo <id> --principal <user> --action <actions>
  kc dataset clone <id> <dir>   # 把交付目录树物化成本地目录
`

type helpEntry struct {
	Description string
	Usage       string
}

var helpEntries = map[string]helpEntry{
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
	"catalog list": {
		Description: "列出可见 Catalog",
		Usage:       `kc catalog list`,
	},
	"catalog use": {
		Description: "选择当前 Catalog",
		Usage:       `kc catalog use <id>`,
	},
	"show": {
		Description: "显示当前 Catalog 的 Repository 与知识集",
		Usage:       `kc show`,
	},
	"create": {
		Description: "创建托管 Repository 或连接自有 Repository",
		Usage: `kc create (--name <名称> [--store <store>] | --url <url> --credential-file <path>)
  --name 与 --url 互斥。--name 是平台供给空仓；--url 是连接已有仓。
  LakeFS 上 --name 就是仓 ID，与 lakeFS 仓库名相同。
  例：kc create --name table-meta`,
	},
	"attach": {
		Description: "把已连接 Repository 挂入当前 Catalog",
		Usage:       `kc attach --repo <id>`,
	},
	"detach": {
		Description: "从当前 Catalog 拿下 Repository",
		Usage:       `kc detach --repo <id>`,
	},
	"grant add": {
		Description: "发放一条仓、Catalog 或 Dataset 范围的授权规则",
		Usage: `kc grant add --principal <id> --action <action[,action...]> (--repo <id> | --catalog <id> [--dataset <id>])
  --action 是 knowledge.read 这种带点的名字，逗号分隔。不能发 '*'（那是 deployment init 的 bootstrap）。
  Dataset 消费权用 --catalog 加 --dataset，动作是 file.read。
  值里有空格或 * 必须加单引号，否则 shell 会把 * 展开成当前目录的文件名。
  重复发放会新增一条规则，不是幂等。撤销抄本条回执或 grant list 的 id。
  例：kc grant add --repo kr://scene/knowledge --principal healzhou --action knowledge.read,knowledge.search
  例：kc grant add --catalog kr://scene/catalog --dataset scene-set --principal healzhou --action file.read`,
	},
	"grant list": {
		Description: "列出授权规则",
		Usage: `kc grant list [--principal <id>] [--action <action>] [--repo <id>] [--catalog <id>]
  不带 principal+action 时列出规则，可按 --repo 或 --catalog 过滤，不是整份 allow 文件。
  两者都给时是一次评价，不是列表。
  例：kc grant list --repo kr://scene/knowledge
  例：kc grant list --repo kr://scene/knowledge --principal healzhou --action knowledge.read`,
	},
	"grant remove": {
		Description: "撤销一条授权规则",
		Usage: `kc grant remove --id <rule-id>
  --id 来自 grant add 回执或 grant list，不是场景执行器占位符。`,
	},
	"catalog audit": {
		Description: "查看 Catalog 登记表历史",
		Usage:       `kc catalog audit [--limit <n>] [--continuation <cursor>]`,
	},
	"catalog archive": {
		Description: "归档当前 Catalog",
		Usage:       `kc catalog archive`,
	},
	"deployment init": {
		Description: "初始化部署",
		Usage: `kc deployment init --config <deployment.yaml>
  --config 是部署文件路径，不是 Catalog id。
  例：kc deployment init --config deployment.yaml`,
	},
	"deployment status": {
		Description: "查看部署恢复状态",
		Usage: `kc deployment status --config <deployment.yaml>
  例：kc deployment status --config deployment.yaml`,
	},
	"deployment system publish": {
		Description: "把 System Schema 写入绑定仓",
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
	"dataset define": {
		Description: "定义可复用的多 Repository 配方（目录映射与逐文件交付）",
		Usage: `kc dataset define <id> --revision <n> (--source <repo>[=<selector>][@path[@subPath]]... | --file <recipe.yaml>)
  --source 可重复；等号后面是 selector，省略则用默认 published。<id> 也可用 --dataset 写出。
  目录映射：第一个 @ 后写交付目录（path），第二个 @ 后写来源仓内的子目录（subPath）。
  所有 --source 都要写 @path（根用 @ 写空串），交付目录不得嵌套重叠；同一仓多个片段共用一个固定 commit。
  逐文件交付：--file-source <repo>[=<selector>]@<来源文件>@<交付路径>，可与 --source 同用；
  同一交付目标文件只能有一个来源，冲突会被拒绝；文件条目不改变所在仓的固定版本。
  例：kc dataset define scene-set --revision 1 --source kr://scene/knowledge
  例：kc dataset define --dataset scene-set --revision 1 --source kr://scene/knowledge
  例：kc dataset define delivery --revision 1 --source docs@reference/policies@handbook/policies --source data@data/metrics@semantic/metrics --file-source docs@handbook/README.md@reference/README.md
  例：kc dataset define scene-set --revision 1 --file .kc-dataset.yaml`,
	},
	"dataset clone": {
		Description: "把已发布 Dataset 的交付目录树物化成普通本地目录",
		Usage: `kc dataset clone <dataset> <dir>
  把已发布 Dataset 的当前服务版交付目录树物化成普通本地目录，不需要 kcfs。
  <dir> 必须是空目录或不存在；clone 不覆盖已有文件。
  回执列出版本、pinId 与每个来源仓的 commit；不写 manifest 文件，也不收 --pin。
  例：kc dataset clone scene-set ./scene-copy`,
	},
	"dataset retire": {
		Description: "退役命名配方",
		Usage: `kc dataset retire <id>
  <id> 也可用 --dataset 写出。
  例：kc dataset retire scene-set`,
	},
	"dataset overlay": {
		Description: "只在本机合成一份不发表的配方",
		Usage: `kc dataset overlay --file <recipe.yaml> --overlay <overlay.yaml> [--out <path>]
  --file 是底稿配方，--overlay 是本机补丁。不发表、不改共享知识集。
  例：kc dataset overlay --file recipe.yaml --overlay overlay.yaml`,
	},
	"schema list": {
		Description: "列出一个 Repository 已发布的实体，不是对象目录或合同正文",
		Usage:       `kc schema list --repo <id> [--limit <n>] [--continuation <cursor>]`,
	},
	"search": {
		Description: "在固定仓或 Dataset 上按 Schema AccessHints 检索知识，列出匹配对象，不是文件 contains",
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
  例：kc read --repo kr://kc/system --object schema/core/readme/v1 --aspect readme`,
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
  查看该实体 Aspect 的 Bound State 声明（Schema origin），不读墙外值。
  例：kc binding show --repo kr://scene/knowledge --object table/orders --aspect stats
  例：kc binding show --dataset scene-set --object table/orders --aspect stats`,
	},
	"access": {
		Description: "按 Schema origin 与实体 ID 读取该 Aspect 的当前观察",
		Usage: `kc access (--repo <id> | --dataset <id>) --object <object-id> --aspect <name>
  访问路径在 Domain Schema frontmatter 的 origin。协议坐标是 origin + 实体 object_id，返回该 Aspect。
  不要加 --operation（那是 invoke）。
  例：kc access --repo kr://scene/knowledge --object table/orders --aspect stats
  例：kc access --dataset scene-set --object table/orders --aspect stats`,
	},
	"invoke": {
		Description: "调用 ResourceDescriptor 操作",
		Usage: `kc invoke (--repo <id> | --dataset <id>) --object <object-id> --operation <name> --input <json>
  --input 是一个 JSON 对象，用单引号包住。
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

var helpGroups = map[string][]string{
	"identity":    {"login", "logout", "whoami", "admission show"},
	"admission":   {"admission show"},
	"composition": {"catalog list", "catalog use", "show", "create", "attach", "detach", "grant add", "grant list", "grant remove", "dataset define", "dataset clone", "dataset retire", "dataset overlay"},
	"catalog":     {"catalog list", "catalog use", "catalog audit", "catalog archive"},
	"dataset":     {"dataset define", "dataset clone", "dataset retire", "dataset overlay"},
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
		return Help, nil
	case "consume":
		return ConsumeHelp, nil
	case "write":
		return WriteHelp, nil
	case "compose":
		return ComposeHelp, nil
	}
	if help := formatLeafHelp(topic); help != "" {
		return help + "\n", nil
	}
	if paths, ok := helpGroups[topic]; ok {
		return formatGroupHelp(topic, paths), nil
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
