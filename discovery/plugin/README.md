# Discovery 的声明式索引消费登记

本包登记真实 `IndexContribution`，按本次 Repo 配置固定选择贡献，路由查询、准备及维护。
它不构造索引引擎，也不承担 Discovery 的查询编排或具体索引内部算法；实际工厂在
[`declarativeindex/plugin`](../../declarativeindex/plugin/README.md)。

公开的 `SearchProjection`、`RelationProjection` 与 `ProjectionDescriber`、
`ProjectionSynchronizer`、`ProjectionController`、`ProjectionStateView` 是已选定贡献的
消费端口；`IndexQueries` 与 `IndexMaintenance` 成组提交这些能力。可选的语义窗口通过
`SemanticWindowProjection` 单独判断，不以登记本身证明支持。

依赖方向：本包只依赖 Discovery 公开查询值、既有 projection 提供方合同、Object 的
Repository/固定读取范围、Snapshot Dataset 收据及 kernel；不依赖 service 执行器、Home、
具体声明式索引工厂、CLI 或物理 driver。固定 Dataset 的发布与准备编排仍归
`discovery/datasets`，其 provider 通过结构相同的 `PrepareServingBasis` 消费本贡献。

共同配置、选择、Scope 和说明事实的实际定义在 [`adoption`](../adoption/README.md)。
本包的旧同名公开类型是别名，解析与分流函数是兼容转发；索引贡献及登记实现继续由本包拥有。

## Repo 插件贡献

`RepositoryPluginsKey`（`kc.plugins`）是原生 Repository metadata 中的一个不含密钥的
配置值；Snapshot 只保存字符串。格式由 `RepositoryPluginConfig` / `PluginSelection`
选定，例如 `{"version":1,"plugins":[{"name":"declarative-index","instance":"main"}]}`。
缺键表示无扩展；显式空数组禁用扩展。非法文档、重复名称、未知字段及不可读 metadata
不当作空配置。`RepositoryPluginConfigs` 在装配时分别保留已读配置和来源原故障：
该来源的扩展任务明确失败，独立来源和连接凭据修复仍可进入，不将故障来源当成未启用。
沿现有 Repo metadata 授权读写；下一次宿主装配读取新配置，本轮不热加载。
物理 URL、凭据与缓存位置只是产品供给的资源，不是另一份启用配置。

[`Home`](../../home/README.md) 的自有连接冷恢复不读取 provider metadata，先保留
明确的待核验故障；连接管理仍可修复凭据。根宿主在启动维护前用一次
`VerifyIndexPlugins` 完成运行装配，之后来源配置与故障固定在该运行视图中。
Handler 冷构造不激活维护；显式 `ManagedHTTPHandler.Start` 或嵌入式首次需要运行资源的
typed 任务完成一次核验，再启动同一个根的贡献。静态插件说明不经过这一步。
请求视图不能核验，metadata 或凭据更新不触发热加载；须重新根装配才采用新配置。

`IndexPluginEntry` 提供静态 `PluginAffordance` 和构造入口，入口收到 `PluginScope` 的
实例身份及明确 Repo 列表，每取得资源立即登记 `Own`。`IndexContribution` 成组提供
`IndexQueries`、`IndexMaintenance` 与启停；`OpenIndexPlugins` 完整校验后才接纳。
`IndexPlugins` 保持仓到实例的固定映射，实际查询、声明依据、准备、维护和诊断都使用
该映射；未配置仓的查询失败，固定 Object 读写独立。它不定义默认实现或优先级。
说明缺失不影响完整运行贡献，`Help` 只返回登记说明，不执行插件。

实例资源只由根宿主持有；请求借用视图不关闭它们。宿主的 `BeginShutdown` 先拒绝新工作，
经根 Home 调用 `StopBackground` 取消并等待后台维护以解除其锁，再排空正在执行的业务 HTTP。
`StopBackground` 不释放资源或拒绝贡献调用，已接受任务的后续步骤仍可使用同一贡献。
排空后 `Close` 拒绝新贡献调用、等待已接受调用，最后逆序释放一次；不取消请求自己的上下文。
构造及激活失败同样清理已取得资源。缺静态入口属于该来源的扩展故障，不阻挡
独立来源或固定 Object 读取，也不回退到其它实例。
搜索、关系及外层 Dataset/遍历续页绑定实际实例；只有接纳既有资源的 `main` 实例
接受旧页，并仍由原提供方完成旧摘要校验。
`Overview` 给出已获准固定来源的 `SourcePlugin` 说明入口，不证明支持、就绪或权限。
Discovery 的 `AccessDescribeExecutor` 将它组合到既有 `AccessPlan`；Schema 内容不增加旁注。
