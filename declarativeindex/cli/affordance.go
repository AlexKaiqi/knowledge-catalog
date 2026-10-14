package cli

import "kc/discovery/adoption"

// PluginAffordance is the module's static product explanation. The plugin
// entry adapts it to the consumer's registration contract without importing
// execution services into this CLI.
type PluginAffordance = adoption.PluginAffordance

// Affordance is reusable static content, registered by the actual plugin entry.
// Reading it does not instantiate an index, inspect readiness or access a source.
func Affordance() PluginAffordance {
	return PluginAffordance{
		Name:    "declarative-index",
		Summary: "按 Schema 字段 access 与类型查询，并准备、维护和恢复固定依据投影。",
		Tasks:   []string{"search", "relations", "traverse", "operations projection describe", "operations projection sync", "operations projection notice"},
		Usage: `声明式索引

声明：
  Schema 字段通过 text/filter/sort 明确开放文本发现、类型化过滤或排序；三种资格互不蕴含。
  查看固定声明用 kc schema describe；字段有歧义时使用 schema::aspect::path。
用法：
  kc search --repo kr://scene/object --query runbook
  kc search --dataset scene-set --eq amount=100 --sort amount:asc
  kc relations --repo kr://scene/object --object metric/gmv
  kc traverse --repo kr://scene/object --object metric/gmv --max-hops 2
条件与保证：
  Repo 配置插件；Dataset 使用本次来源仓的贡献及固定范围，不另安装插件。
  先验证声明资格、提供方组合能力、固定版本就绪及当前授权；声明本身不证明能够执行。
  命中和完整正文按同一依据回读，续页保持原范围与版本；动态命中读取原观察。
  不支持、未就绪、坏坐标、依据或回读失败明确报错，不能当作完整零命中。
  覆盖、近似或预算限制如实报告 partial。Object 固定读取与基本发布不等待索引。
维护与恢复：
  Writer 接受提交后提示后台维护；遗漏通知、重启或物理索引丢失由对账恢复，派生失败不撤销源提交。
  查询和说明读取不现场建索引；排障用 kc operations projection describe，显式维护用 sync/notice。
  新 Dataset 查询版本全部必需准备完成后才接受；失败保留旧发布版。
边界：
  说明只读，不访问引擎或来源，不发权、不展示实例配置，不代表当前支持或就绪。
  OpenSearch 是物理 driver，业务任务与声明不随 driver 更换改名；每个任务的参数见 kc help <任务>。`,
	}
}
