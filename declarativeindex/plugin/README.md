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

具名 Dataset 的托管观察由 `RevisionReferenceCoordinator` 与已有发布权威协调。每个插件实例
使用稳定 owner，先取得实际权威基点，再准备并保护精确生成代，持久准入后以原基点接受当前观察引用
和不可变收据，最后确认资源。引用内保存当前证据及被替换观察的原续页截止时间；后续刷新只保留
未到期项，不顺延期限。技术 ROOT 的推进不能替换实际接受引用或启动旧观察的宽限。

查询只读实际引用，不执行围栏或修复；保留修订的最后观察仍随原修订可消费，退役拒绝。资源恢复
识别观察准入，先在读取准入之后建立实际发布围栏，排除仍暂停的原 CAS，再按独有收据判断接受事实。
绑定不符、坏证据或未知权威错误不给释放结论，收据事实不等同于后续生命周期仍允许消费。
最终资源释放、检索保留及紧急清除仍按主任务交付，不由该事实恢复替代。

已接受引用对账通过只读 `PublicationInspector`，与未决准入的围栏恢复分开。静态引用核对
实际 Dataset 生命周期；观察引用还核对实际当前证据及原续页截止。旧观察到期或已知 Dataset
退役才能解除该引用，接受收据仍保留事实；未知和缺失生命周期不给释放结论。每轮对账不推进
发布权威，也不让维护把已释放的精确准入身份恢复成新引用。
