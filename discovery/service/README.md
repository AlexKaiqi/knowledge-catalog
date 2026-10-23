# Discovery 任务服务

`SearchExecutor` 与 `DatasetSearchExecutor` 在固定版本和范围上查询现有可信投影；`DatasetRelationsExecutor`、`DatasetTraverseExecutor`、`RepoTraverseExecutor` 执行关系导航。正式 Dataset 关系查询仅向固定 corpus 发起一次物理查询，遍历的每一跳复用该执行器；低层无 Dataset scope 的提供方仍按原固定来源合同运行。所有命中经相同依据回读与交付，消费路径不修复投影、不扫描权威。`AccessDescribeExecutor` 描述 Schema 字段承诺：装有插件时按 `DeclaredAccessSpecAt` 带出选入声明的用途与成员状态，不执行引擎操作；未装插件或成员无实例时只给结构。Dataset 搜索因字段缺少用途被拒时，错误给出 `kc schema describe --dataset <id> --revision <N>` 作为恢复入口。投影 describe/sync 沿既有明确维护任务。

端口由消费方定义，宿主注入真实实现。本包不打开部署、解析 argv 或注册传输入口。导出形状由 `surface_anchor_test.go` 保持闭集。当前可信索引的 scope/budget 接线不代表任意实现的隔离保证。

`ProjectionNoticeExecutor` 将显式 runtime 及请求依据交给 Controller 的本次 notice 执行口，
不调用后台来源配置 setter。响应采用这次实际发布的 `StateSync`，不再次跟随 HEAD；
未提供前台 runtime 的通知继续沿已有后台队列执行。

`DatasetPrepareExecutor` 是显式准备的执行口：先让拥有全部来源的实例构建该已接受修订的
corpus，再用本次调用方的 runtime 重新发布这次 corpus 的观察。调用方只交出 runtime 与
请求依据工厂，把 runtime 绑成观察来源只在本包发生，调用层不直接依赖 LiveAccess 的桥接
适配器。进程已有后台来源时仍用后台来源，前台 runtime 只在没有后台来源时兜底。

已安装的声明式索引实例通过 Dataset corpus 端口执行消费查询，不再按来源仓拼接排名。
`DatasetSearchExecutor` 先授权，再固定修订；有外部观察的候选在本次逻辑 READ 后、填满
产品页之前补判。组合对象沿同一版本向量回读各来源的选入方面。索引原观察与 READ 新
观察分别保留，前台补判不改变共享观察代；没有 Dataset corpus 端口的底层提供方夹具
仍通过固定来源合同证明相同的预算、回读与分页风险。

来源对象只有部分单元选入 Dataset 时，消费回读先从固定维护坐标规划实际交付的 Address，
再通过已有逻辑引用读取逐个取回完整单元并组装。完整来源继续沿普通 READ；未选兄弟
不会参与回读，外部单元的值、逐值失败和观察依据仍由同一逻辑读取器提供。组合命中
只使用命中所带、且与本次 Dataset 修订完全一致的真实来源版本。

宿主的交付端口沿本次已解析的完整 Serving 保存对象读取依据，并把不透明引用及本地状态放入
命中对象；它不能仅以实际贡献者作为读取范围。本包不拥有依据记录库，准备与查询仍消费对象层
的固定坐标合同。查询结构不可用时，仍有效的对象依据继续经独立读取入口重新鉴权与回读，
不因此恢复查询权限或原外部观察。


仓到插件实例的登记、运行贡献与生命周期由 [`discovery/plugin`](../plugin/README.md) 拥有。
本包执行器消费该包的窄查询和维护端口；`AccessOverview` 保留原 `AccessPlan` 字段，另给
获准固定来源的 `SourcePlugin` 说明入口，不证明支持、就绪或权限。
