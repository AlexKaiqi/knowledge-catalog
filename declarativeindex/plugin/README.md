# declarativeindex/plugin

`Entry(Options)` 是声明式索引的真实构造与说明登记入口。宿主按部署安装的实例传入
`PluginScope`，入口组装实际 Index、Controller、固定 Dataset 准备与恢复贡献；
实际查询和派生发布归本模块 `projection/`，调度及恢复归 `control/`。

静态说明来自所属 `declarativeindex/capability`，读取 `Affordance` 不调用 `Open`、访问来源、
探测引擎或启动维护。构造时每取得 Index 立即登记 `Own`；完整登记后宿主才激活。
入口构造不启动 Controller；`Start` 先核验完整有效需求，再启动维护，初始需求读取失败
沿原错误拒绝激活。`Stop` 只取消并等待维护，
保留 Index 供正在执行的业务 HTTP 完成；资源由宿主在请求排空后通过 `Own` 释放一次。

入口按宿主当前接纳的来源限制 lookup 与维护库存；没有当前集合端口时保持构造时的固定范围，Dataset 不另建实例；同实例的多个来源各自追平，
`CatchUpRepository` 不跨来源执行维护。稳定 `main` 接纳既有目录与
物理 namespace，其它实例使用明确的目录和 namespace；不改变对象身份。
当前未退役 Dataset 的固定来源经 `ServingBasisDemand.Bases` 交给控制器，HEAD 不可读
也不阻断合法固定版本恢复。工作账不是需求来源，空分区不会执行旧任务。
部署实例配置、一次根运行装配与关闭合同见
[`Discovery 消费登记`](../../discovery/plugin/README.md) 和
[`Home`](../../home/README.md)。本包不处理授权、安装管理或热加载。


`Options.Manifest` 是此实例配置的清单；入口固定独立副本，同一清单同时提供 `AffordanceFor`、
逐成员 `Check` 和 `Rebuilds`，并经 typed `IndexCapabilities` 公开，不能以编译默认代替实例差异。
实例清单摘要与 driver 物理摘要共同进入投影的 `PhysicalDigest`；清单变化使旧投影不再证明就绪，
即使该成员的逻辑支持结果没变。`AccessDigest` 仍标识逻辑字段与声明。
