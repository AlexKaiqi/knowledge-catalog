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
发布        把目录里的最终样子发布到一个 Repository
治理（进阶）校验并合入 Proposal
运维        投影、Hook、Gate 与审计
部署        初始化、恢复并运行 Server

最短旅程：kc help consume | kc help write | kc help compose
仓与文件交付：kc help composition | kc help dataset | kc help files
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
  kc schema list --repo <id>
  kc read <对象URN[#/locator]>
  kc search --repo <id> --query 'merchandise'

已定义 Dataset：
  kc search --dataset <id> --query 'merchandise'
  kc read --dataset <id> <对象URN[#/locator]>
回执带 repository / objectId / commit。不要先造一份 pin 文件。
`

const writeHelp = `kc help write — 创建并发布一个 Repository

  kc create --name <名称>
  # 或：kc create --url <repository-url> --credential-file <path>
  kc writer commit --command-id <id> --repo <id> --dir <drafts>
  kc attach --repo <id>
  kc grant add --repo <id> --principal <user> \
    --action object.read,object.search,object.schema.read

提交前可看：kc diff --repo <id> --dir <drafts>
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
	hostGovernanceHelpEntries(),
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
	"governance proposal create", "governance preview create",
	"governance validation record", "operations hook add", "operations gate add",
	"operations feedback record", "deployment init", "deployment status",
	"deployment system publish", "serve",
})))))

var helpGroups = composeLiveAccessHelpGroups(composeDeclarativeIndexHelpGroups(composeDiscoveryHelpGroups(composeObjectHelpGroups(composeSnapshotHelpGroups(map[string][]string{
	"identity":    {"login", "logout", "whoami", "admission show"},
	"admission":   {"admission show"},
	"composition": {"create"},
	"object":      {},
	"publishing":  {"diff", "writer commit", "writer head", "writer receipt"},
	"writer":      {"writer commit", "writer head", "writer receipt"},
	"governance":  {"governance proposal create", "governance preview create", "governance preview validate", "governance validation record", "governance proposal merge"},
	"operations":  {"operations hook add", "operations hook list", "operations hook remove", "operations gate add", "operations gate list", "operations gate remove", "operations audit access", "operations audit trace", "operations audit hitmap", "operations feedback record"},
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
