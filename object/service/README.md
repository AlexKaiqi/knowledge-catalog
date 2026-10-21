# Object 任务服务

Schema 浏览与固定历史分页由 `SchemaBrowseExecutor`、`ObjectLogExecutor` 拥有，其续页绑定原依据；传输只提供当前权限和固定读口。固定读取与对象写入分别由 `ReadExecutor`、`CommitExecutor` 执行。基础读取只接受 Object `Reference`；来源身份不再分列，直接读取另给固定 commit。Dataset 读取由 `DatasetReadExecutor` 先检查当前授权，再经注入的 `ReferenceRead` 读取已接受发布范围内的来源对象并交付，基础任务不依赖当前态或发现实现。请求使用 Object/Writer 的类型，不接收 argv、HTTP 请求或部署实例。

导出形状由 `surface_anchor_test.go` 保持闭集，原用例与当前权限和版本保证随实现迁移。

`MaintenanceExecutor` 将统一 `MaintenanceRequest.Reference` 在固定仓基点解析为完整维护
Address，再进入已有 `CommitExecutor`／Writer。`ResolveMaintenanceAddress` 是授权和执行共用
的唯一维护模型判断，不从 JSON 正文或路径深度猜 Aspect／Member。新对象的 Kind 显式声明，
现有 Relation 的种类保留；草稿目录批次和 Dataset 消费授权不转为单对象写权。命令重放采用
原回执的固定仓基点，仍由 Writer 核对内容与 CAS，不重发不确定结果。
只提供 Expected 时，省略的 Base 沿用该预期提交来解析维护单元；两者都省略才采用当前
Head。过期预期仍由 Writer／Snapshot 的 CAS 拒绝，不改成两种基点不一致的参数错误。

`DatasetReadExecutor.ReferenceRead` 是必需的逻辑读取接缝：当前授权先于回读，来源必须属于已
接受的 Dataset，来源版本与范围保持固定。宿主注入逻辑读取实现；Object 服务不解释 Binding
或调用 State runtime。完整声明先经限定读取，外部观察应用后才提取所选 leaf。

`ShapeReadExecutor` 把 `ShapeReadRequest.Shape`（GraphQL selection set）编译成方面／成员级引用，
每条经注入的 `Read` 走一次已有的引用读取：先按维护坐标规划，只读所选单元，只为所选单元外部
取值；字段在单元读出后即裁剪，不先整读对象再删。无权、冲突、缺失按路径进 `errors`，其余失败
整批拒绝。`AdoptedSchema` 只在 `Schema` 端口给出的请求范围内找所选单元采用的 Schema，找不到
按开放处理；URN 已带定位符时拒绝形状。
