package capability

import (
	"fmt"
	"kc/discovery/adoption"
	"strings"
)

// PluginAffordance is the module's static product explanation. The plugin
// entry adapts it to the consumer's registration contract without importing
// execution services into this CLI.
type PluginAffordance = adoption.PluginAffordance

// Affordance is reusable static content, registered by the actual plugin entry.
// Reading it does not instantiate an index, inspect readiness or access a source.
func Affordance() PluginAffordance { return AffordanceFor(DefaultManifest()) }

// AffordanceFor explains the same configured manifest that the instance checks.
func AffordanceFor(manifest Manifest) PluginAffordance {
	return PluginAffordance{
		Name:    "declarative-index",
		Summary: "解释 Dataset 选入的索引声明，按实例能力清单检查，并维护固定依据投影。",
		Tasks:   []string{"plugin declarative-index help", "plugin declarative-index describe", "plugin declarative-index trial", "search", "relations", "traverse", "operations projection describe", "operations projection sync"},
		Usage: `声明式索引

声明：
  每个采用 kc/index-declaration 的成员声明 inputs、build、uses 与 catalog（能力清单名@版本）。
  Schema 只描述结构（kc schema describe）；uses 可选 text/filter/sort/semantic。
  Writer 检查形状与 Schema 字段定位符，实例逐成员核对清单支持情况。
  读实际部署清单：kc plugin declarative-index describe；逐成员试运行：
  kc plugin declarative-index trial --repo <id> --file <声明文件> [--commit <固定版本>]
  trial 返回 supported/unsupported/invalid、原因与是否重建，不写入对象、命令账本或索引。
用法：
  kc search --dataset scene-set --revision 1 --query runbook
  kc search --dataset scene-set --eq amount=100 --sort amount:asc
  kc relations --dataset scene-set --revision 1 --object example/report
  kc traverse --dataset scene-set --revision 1 --object example/report --max-hops 2
条件与保证：
  部署安装实例并把清单名映射到唯一实例；选入 Dataset 的声明决定检索字段，不另安装插件。
  先验证声明资格、提供方组合能力、固定版本就绪及当前授权；声明本身不证明能够执行。
  命中和完整正文按同一依据回读，续页保持原范围与版本；动态候选按本次读取的新观察复核，保留索引原观察。
  不支持、未就绪、坏坐标、依据或回读失败明确报错，不能当作完整零命中。
  覆盖、近似或预算限制如实报告 partial。Object 固定读取与基本发布不等待索引。
维护与恢复：
  来源提交与 Dataset 发布独立；来源推进不改变已发布修订。当前 Dataset 索引丢失后由维护者显式同步恢复，自动恢复尚未就绪。
  查询和说明读取不现场建索引；排障用 kc operations projection describe --dataset <id>，有 dataset.manage 的维护者显式 sync 同一修订。
  新 Dataset 查询版本全部必需准备完成后才接受；失败保留旧发布版。
边界：
  说明只读，不访问引擎或来源，不发权、不展示实例配置，不代表当前支持或就绪。
  OpenSearch 是物理 driver，业务任务与声明不随 driver 更换改名；每个任务的参数见 kc help <任务>。
` + capabilityHelp(manifest),
	}
}

func capabilityHelp(manifest Manifest) string {
	var out strings.Builder
	fmt.Fprintf(&out, "编译清单（实际安装实例以 describe 为准）：%s，摘要 %s\n", manifest.ManifestCatalog(), manifest.Describe().Digest)
	for _, build := range manifest.Builds {
		fmt.Fprintf(&out, "  %s（%s）：输入 %s，最多 %d 项，用途 %s\n", build.Kind, build.Category, strings.Join(build.InputTypes, "/"), build.MaxInputs, strings.Join(build.Uses, "/"))
		if build.UnavailableReason != "" {
			fmt.Fprintf(&out, "    当前不可用：%s\n", build.UnavailableReason)
		}
		for _, option := range build.Options {
			values := option.Type
			if option.Values != nil {
				values = strings.Join(option.Values, "|")
				if values == "" {
					values = "未登记可选值"
				}
			}
			fmt.Fprintf(&out, "    %s=%s；修改是否重建：%t\n", option.Name, values, option.Rebuild)
			if option.UnavailableReason != "" {
				fmt.Fprintf(&out, "      当前不可用：%s\n", option.UnavailableReason)
			}
		}
	}
	for _, input := range manifest.Inputs {
		fmt.Fprintf(&out, "  input.%s (%s)；修改是否重建：%t\n", input.Name, input.Type, input.Rebuild)
		if input.Values != nil {
			fmt.Fprintf(&out, "    可选值：%s\n", strings.Join(input.Values, "|"))
		}
		if input.UnavailableReason != "" {
			fmt.Fprintf(&out, "    当前不可用：%s\n", input.UnavailableReason)
		}
	}
	fmt.Fprintf(&out, "  上限：inputs=%d，maxTokens=%d；不开放：%s\n", manifest.Limits.MaxInputs, manifest.Limits.MaxTokens, strings.Join(manifest.Closed, ", "))
	return out.String()
}
