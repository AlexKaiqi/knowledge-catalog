# Object 维护

`WalkRepository`、`WalkSnapshot`、`WalkScope`、`WalkFileScope` 提供有界固定版本遍历；`ChangedObjectIDs` 解释 Snapshot 变更涉及的业务对象。维护端口独立于消费 Reader，消费读不会因未就绪而扫描或修复。

`Coordinator` 为独立 `SnapshotConsumer` 保存固定目标及进度，按各消费者自己的恢复路径对账。消费者登记在启动时冻结；`Desire`、`CatchUp` 与后台协调共用同一进度合同。缓存及投影分别实现消费者，协调器不依赖具体缓存、索引或动态来源。

实现与公共入口同属本包，没有深层转发层。只处理对象及 Snapshot 的固定范围，不访问动态来源，不解释搜索就绪。
