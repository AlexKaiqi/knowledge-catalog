# 声明式索引能力说明

本包提供真实 declarative-index 插件可复用的静态 Affordance 与能力清单。插件入口登记 Affordance()；
维护任务入口来自 [`declarativeindex/cli`](../cli/README.md) 的 Commands()，本包只引用其名称，不复制叶规则。

`Affordance()` 返回纯数据 `PluginAffordance`：Name、Summary、Usage、Tasks。
它与说明文字、任务入口同源；插件 Entry 映射到消费方的登记合同，不复制内容。

`Manifest` 的公开数据采用 Discovery 检索合同；`DefaultManifest` 与实际 Entry 使用同一份
版本、可选值、重建规则和上限。`Describe` 返回独立副本并重算摘要；公开 JSON 往返不依赖
私有索引才能判断。`Check` 只判断实例能力，Writer 仍接纳形状正确但实例不支持的声明。
`Rebuilds` 遵循各项的重建规则：输入截断与构建参数按清单重建，打分权重可以无需重建。
默认实例的 text 模板以 `{字段路径}` 引用已声明的输入；未知或同名歧义输入明确不支持。
keyword 的 `lowercase` 对构建值和查询值应用相同规范化；数字输入可构建为 keyword。
`missing` 的 `skip` 保留其他输入，`exclude` 只撤下该对象对应的成员构建，不影响其他成员。
vector 只接纳部署实际登记的模型与已实现的 cosine 距离；没有登记模型的实例明确不支持。
`AffordanceFor` 展示配置清单；没有运行实例的静态说明明确导航到实际 `describe`。
清单与帮助不开放原始 mapping、脚本、分片或副本设置。
统一 `plugin ... describe/trial` 任务及认证传输归 Discovery CLI/SDK；本包不接管实例选择或安装管理。
