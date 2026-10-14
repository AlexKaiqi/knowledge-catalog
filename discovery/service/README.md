# Discovery 任务服务

`SearchExecutor` 与 `DatasetSearchExecutor` 在固定版本和范围上查询现有可信投影；`DatasetRelationsExecutor`、`DatasetTraverseExecutor`、`RepoTraverseExecutor` 执行关系导航。所有命中经相同依据回读与交付，消费路径不修复投影、不扫描权威。`AccessDescribeExecutor` 描述 Schema 字段承诺；投影 describe/sync/notice 沿既有明确维护任务。

端口由消费方定义，宿主注入真实实现。本包不打开部署、解析 argv 或注册传输入口。导出形状由 `surface_anchor_test.go` 保持闭集。当前可信索引的 scope/budget 接线不代表任意实现的隔离保证。

`ProjectionNoticeExecutor` 将显式 runtime 及请求依据交给 Controller 的本次 notice 执行口，
不调用后台来源配置 setter。响应采用这次实际发布的 `StateSync`，不再次跟随 HEAD；
未提供前台 runtime 的通知继续沿已有后台队列执行。


仓到插件实例的登记、运行贡献与生命周期由 [`discovery/plugin`](../plugin/README.md) 拥有。
本包执行器消费该包的窄查询和维护端口；`AccessOverview` 保留原 `AccessPlan` 字段，另给
获准固定来源的 `SourcePlugin` 说明入口，不证明支持、就绪或权限。
