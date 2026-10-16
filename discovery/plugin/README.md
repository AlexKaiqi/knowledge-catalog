# Discovery 的声明式索引消费登记

本包登记真实 `IndexContribution`，打开部署安装的索引实例，路由查询、准备及维护。
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

`DatasetMember` 只向所选实例的同名只读端口委派；固定范围的声明命名空间由提供方解释，
登记本身不构造对象视图。没有这个端口的实例明确报告缺能力。

共同配置、Scope 和说明事实的实际定义在 [`adoption`](../adoption/README.md)。
本包的同名公开类型是别名；索引贡献及登记实现继续由本包拥有。

## 部署索引实例

`OpenIndexPlugins(config, scope, entries)` 只读部署的 `PluginsConfig`，开头调用
`CheckCatalogs`，一个清单名只对应一个实例；索引实例必须恰好声明 `Catalog`。
没有来源上的选择配置：Repo 或 Dataset 不决定由谁解释。在声明按清单名路由之前
（INDEX-SEARCH-SPACE-01），唯一安装的索引实例解释部署的全部来源；多于一个实例时
拒绝启动，不猜测。未安装索引插件时查询明确报没有能力，固定 Object 读写不受影响。
部署配置在根装配时读取，本轮不热加载。

[`Home`](../../home/README.md) 的自有连接冷恢复不联系来源。根宿主在启动维护前用一次
`VerifyIndexPlugins` 按当时的全部来源重新打开实例，之后安装的实例固定。来源准入与维护库存通过宿主并发安全的当前集合读取；新增或移除来源不重开实例，成员关系不代替调用者授权。
Handler 冷构造不激活维护；显式 `ManagedHTTPHandler.Start` 或嵌入式首次需要运行资源的
typed 任务完成这一步，再启动同一个根的贡献。静态插件说明不经过这一步。

`IndexPluginEntry` 提供静态 `PluginAffordance` 和构造入口，入口收到 `PluginScope` 的
实例身份、来源初始列表及宿主的当前准入和库存端口，每取得资源立即登记 `Own`。`IndexContribution` 成组提供
`IndexQueries`、`IndexMaintenance`、实例的只读 `IndexCapabilities` 与启停；`OpenIndexPlugins` 完整校验后才接纳。
`IndexPlugins` 保持部署解释器不变；固定来源列表或宿主当前准入端口共同限定可参与的来源，实际查询、声明依据、准备、维护和诊断采用同一准入边界。Dataset 的全部来源先检查，再调用提供方。实例被多个来源共享，所以 `IndexMaintenance.CatchUpRepository` 只追平指定来源，
不得顺带执行同实例其它来源的维护。它不定义默认实现或优先级。
说明缺失不影响完整运行贡献，`Help` 只返回登记说明，不执行插件。

实例资源只由根宿主持有；请求借用视图不关闭它们。宿主的 `BeginShutdown` 先拒绝新工作，
经根 Home 调用 `StopBackground` 取消并等待后台维护以解除其锁，再排空正在执行的业务 HTTP。
`StopBackground` 不释放资源或拒绝贡献调用，已接受任务的后续步骤仍可使用同一贡献。
排空后 `Close` 拒绝新贡献调用、等待已接受调用，最后逆序释放一次；不取消请求自己的上下文。
构造及激活失败同样清理已取得资源。部署安装了未编译的入口时只是不装该能力，
不阻挡固定 Object 读取，也不回退到其它实例。
搜索、关系及外层 Dataset/遍历续页绑定实际实例；只有接纳既有资源的 `main` 实例
接受旧页，并仍由原提供方完成旧摘要校验。
`Overview` 给出已获准固定来源的 `SourcePlugin` 说明入口，不证明支持、就绪或权限。
Discovery 的 `AccessDescribeExecutor` 将它组合到既有 `AccessPlan`；Schema 内容不增加旁注。
`DeclaredAccessSpecAt` 用该仓所属实例的清单检查编译此 commit 选入的索引声明，
不触碰引擎、投影或就绪探测，供解释类描述使用；能否立即检索另由投影就绪回答。
`AccessSpecAt` 则经已维护投影给出检索实际采用的合同。


`CapabilityManifests(name)` 返回已安装实例的清单、版本、摘要与部署映射。
`DeclarationCapabilities(name, catalog)` 按声明清单名交给已安装解释器；版本不符保留原声明交由
解释器报告，不替换声明版本。typed `IndexCapabilities` 只承载清单说明、支持与重建判断，
没有任意任务名或运行角色字典。试运行的 Writer 校验由 Home 编排，通用登记不依赖 Writer。
