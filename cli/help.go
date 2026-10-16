package cli

import (
	"fmt"
	objectcli "kc/object/cli"
	"sort"
	"strings"
)

// Root help includes the Object model; leaf parameters remain in leaf help.
var rootHelp = `kc — Knowledge Catalog CLI

身份        登录并查看当前身份与已有权限
组合        查看登记空间、挂接 Repository、发权
对象        发现对象类型、搜索和读取对象
发布        创建和修订对象，发布到自己的 Repository
分支        在工作分支写入，校验后条件合并
运维        投影、Hook 与审计
部署        初始化、恢复并运行 Server

最短旅程：kc help consume | kc help write | kc help compose
已有例子与首次发布：kc help write（先读公开 Dataset，再复用 Schema 与文件格式）
下载文件：kc help files（按仓和 commit，或按 Dataset）
仓与文件交付：kc help composition | kc help dataset
文件与固定版本入门：kc help snapshot | kc help versions
配方与自定义属性：kc help recipe | kc help metadata
授权与登记历史：kc help grant | kc help registry
只读文件视图：kcfs help
命令帮助：kc help <组> [<动词>]
对象访问概览：kc help operations access-spec describe
读取插件说明：kc plugin <名称> help；本地查看调用方法：kc help plugin
` + "\n" + objectcli.ModelHelp()

const consumeHelp = `kc help consume — 连接服务并读取对象

第一次使用先取得服务地址与该部署支持的登录方式：kc help login。
要消费原始文件，走 kc help files；要理解来源仓与固定交付范围，走 kc help snapshot。

  kc login
  kc show
  kc dataset show <dataset-id>
  kc files list --dataset <dataset-id> --revision <返回的revision>
  kc files read --dataset <dataset-id> --revision <同一revision> --path <entries.path> --out <新文件>

先按 kc show 返回的用途选择 Dataset，沿 entries.path 读导读、对象与 Schema 原文件。
后续列目录、翻页和读取均沿用同一 revision；--out 目标必须不存在。
公开例子可从 qinghe-analysis、autumn-launch-kit、store-oncall 开始，以当前库存可见范围为准。
按对象消费时同样沿用 dataset show 返回的 revision，各步看到同一组来源 commit：
  kc schema list --dataset <id> --revision <N>
  kc schema describe --dataset <id> --revision <N> --object <schemas[].objectId>
  # fields[].access 说明可检索字段与用途（text 供 --query，filter 供 --eq，sort 供 --sort），由选入的索引声明计算；Schema 源文件不写 access。search.state 说明是否就绪。
  kc search --dataset <id> --revision <N> --query '<关键词>'
  kc read --dataset <id> --revision <N> <hits[].objectId 的对象URN[#/locator]>
  kc relations --dataset <id> --revision <N> --object <objectId>
  kc traverse --dataset <id> --revision <N> --object <objectId> --max-hops 2
有来源仓读取权时，也可用 kc schema list --repo <id> 查看类型，再按已知 URN 读取。
回执带 repository / objectId / commit，搜索结果的 dataset.revision 是实际所用修订。不要先造一份 pin 文件。
definition 中的文件地址是普通值；按业务说明取得资源自己的仓、commit 和路径后，用 kc files read 下载。
资源可在别仓、使用独立版本，不默认套用对象说明的 commit。调用方式和 file.read 条件：kc help files。
`

const writeHelp = `kc help write — 复用已有说明，把资料发布到自己的 Repository

1. 先读当前服务公开的例子，取得同一版的 Schema 与对象文件。
  kc show
  kc dataset show <dataset-id>
  kc files list --dataset <dataset-id> --revision <返回的revision>
  kc files read --dataset <dataset-id> --revision <同一revision> --path <entries.path> --out <新文件>
  # qinghe-analysis 是销售分析，autumn-launch-kit 是上新素材，store-oncall 是值班交接。
  # 沿导读和目录选择材料，查看对象的 schema_ref，并读取对应 Schema 原文件；不要照抄读取响应信封。
  # 完整复用旅程：kc read 'urn:kc:object:kc/guide/first-publication#/guide'

2. 选择自己的目标仓与写入方式。已有仓直接使用；只有尚无目标仓时才看 kc help create。
  # 一个完整 JSON 值：kc writer put <对象URN[#/locator]> --repo <id> --command-id <id> --file <value.json>
  # 多个带文件头的对象文件，或 Schema 与实例一起发布：
  kc diff --repo <id> --dir <drafts>
  kc writer commit --command-id <id> --repo <id> --dir <drafts>
  # 目录文件格式、完整通用样例与校验规则：kc help writer commit。
  # 单值输入、完整替换与并发条件：kc help writer put。diff 不替代发布校验。

3. 按接受回执的 result.newCommit 显式读回 Schema 与对象，核对内容、身份和来源。
  kc read <对象URN[#/locator]> --repo <id> --commit <newCommit>
  # 已有仓不重复 create/attach；确需登记时看 kc help attach，交付与授权看 kc help compose。
`

const composeHelp = `kc help compose — 组合 Repository 并发权

Repository 保存来源文件和自己的 commit 历史；Dataset 引用其中固定版本的文件，另有发布 revision。
示例：docs 的 handbook/ 交付成 delivery 的 reference/，不复制出一个新的可写仓。

1. 取得服务地址和身份，按该部署支持的方式登录；发现已登记来源。
  kc help login
  kc show
  # 以下假设来源身份是 docs，来源管理者已确认其默认分支与 handbook/ 路径。
  # 未连接/未登记时，分别看 kc help create / kc help attach，不能因 show 中缺仓就重复创建。
  # 已在 show 中看到 docs 时跳过 attach；下面只在已连接但未登记时执行。
  kc attach --repo docs

2. 维护者发布范围。以下 delivery 第 1 版仅用于尚未使用该版号的 Dataset。
  kc dataset define delivery --revision 1 --source 'docs=refs/heads/main@reference@handbook' --metadata '{"description":"项目材料"}'
  kc dataset show delivery --revision 1
  # 核对 sources 中的实际 commit、revision 和属性；以后源仓推进不改变这份发布。
  # 中断后可原样重跑同一条 define：已接受时回答 disposition REPLAYED；同版号换配方仍拒绝。
  # selector 合法形式和来源版本取得办法：kc help versions；完整配方：kc help recipe。

3. 授权管理者给消费者文件权；发布并不隐含发权，已有所需授权可跳过新增规则。
  kc grant add --dataset delivery --principal consumer \
    --action dataset.resolve,file.read
  # 查看库存/定义另需 inventory.read，管理 Dataset、来源或授权分别需要相应管理权。

4. 消费者在自己的已登录身份下选文件。使用真实返回的版号与 entries.path。
  kc files list --dataset delivery --revision 1 --path reference --limit 20
  kc files read --dataset delivery --revision 1 --path reference/index.md --out ./index.md
  # 假设 handbook/ 中实际存在 index.md；--out 目标必须不存在。
  # 若要整个当前交付树：kc dataset clone delivery ./delivery-copy（目标不存在或为空）。

发布可能独立保存配方，后续失败不回滚已完成写回；丢响应先查原 revision。
单文件下载完成后才发布新目标；clone 不保证整树回滚，重试可能采用新当前版。
阅读每一步的叶帮助核对前提和恢复；分页/历史文件消费详见 kc help files。
Repository 与 Dataset 的元数据管理边界见 kc help metadata。
`

type helpEntry struct {
	Description string
	Usage       string
}

var helpEntries = mergeHelpEntries(
	snapshotHelpEntries(),
	objectHelpEntries(),
	discoveryHelpEntries(),
	declarativeIndexHelpEntries(),
	liveAccessHelpEntries(),
	hostEnvironmentHelpEntries(),
	hostWriterHelpEntries(),
	hostOperationsHelpEntries(),
)

func mergeHelpEntries(groups ...map[string]helpEntry) map[string]helpEntry {
	entries := make(map[string]helpEntry)
	for _, group := range groups {
		for path, entry := range group {
			if _, exists := entries[path]; exists {
				panic(fmt.Sprintf("duplicate CLI help entry %q", path))
			}
			entries[path] = entry
		}
	}
	return entries
}

// Host value-writing rules and every module Affordance are checked separately
// from the root/group navigation; module leaf names come from its declarations.
var complexLeafOperands = includeLiveAccessValueHelp(includeDeclarativeIndexValueHelp(includeDiscoveryValueHelp(includeObjectValueHelp(includeSnapshotValueHelp([]string{
	"login", "create",
	"diff", "writer commit",
	"writer merge", "operations hook add",
	"operations feedback record", "deployment init", "deployment status",
	"deployment system publish", "serve",
})))))

var helpGroups = composeLiveAccessHelpGroups(composeDeclarativeIndexHelpGroups(composeDiscoveryHelpGroups(composeObjectHelpGroups(composeSnapshotHelpGroups(map[string][]string{
	"identity":    {"login", "logout", "whoami", "admission show"},
	"admission":   {"admission show"},
	"composition": {"create"},
	"object":      {},
	"publishing":  {"diff", "writer commit", "writer branch", "writer merge", "writer head", "writer receipt"},
	"writer":      {"writer commit", "writer branch", "writer merge", "writer head", "writer receipt"},
	"operations":  {"operations hook add", "operations hook list", "operations hook remove", "operations audit access", "operations audit trace", "operations audit hitmap", "operations feedback record"},
	"deployment":  {"deployment init", "deployment status", "deployment system publish", "serve"},
})))))

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
	if text, ok := snapshotHelpTopics[topic]; ok {
		return "kc help " + topic + "\n\n" + strings.TrimSpace(text) + "\n", nil
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
	return "", fmt.Errorf("unknown help topic %s; want consume, write, compose, a concept or a command group; run kc help to discover task, concept and command topics", topic)
}

func formatGroupHelp(topic string, paths []string) string {
	lines := []string{"kc help " + topic, ""}
	if text := strings.TrimSpace(snapshotHelpTopics[topic]); text != "" {
		lines = append(lines, text, "", "命令：")
	}
	for _, path := range paths {
		description := helpEntries[path].Description
		if description == "" {
			description = "进阶命令；运行叶子 help 查看操作数"
		}
		lines = append(lines, fmt.Sprintf("  kc %-42s %s", path, description))
	}
	return strings.Join(lines, "\n") + "\n"
}
