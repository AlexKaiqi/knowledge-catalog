# LiveAccess 原结果证据

本包保留消费者读取实际采用的动态值、合法 null 或单值失败，以及固定声明和原观察依据。
Snapshot 内容仍以固定版本及位置复核；访问过程账只保存本包的 `Reference`，不复制实际载荷。
审计授权和按请求定位由应用服务负责；本包不访问来源，也不把保留结果变成对象。

- `Sample` 区分 `ValueOutcome` 与 `FailureOutcome`。成功必须有同地址、固定 commit 和声明摘要的合法 `UnitObservation`；失败只含原因，不含值或成功观察。合法 null 明确编码，缺少值不能当作 null。
- `NewCollector` / `WithCollector` / `Capture` 只在消费请求中捕获已经采用的结果。调用方在选择及核验后捕获，未采用单元不登记；无 Collector 的投影维护不形成消费证据。`Values` / `CapturedValues` 返回独立副本及复制错误。
- `NewFileStore(directory, retention)` 的保留期限必须显式提供；宿主在记录前注入现有政策的 `MaxBytes`，使用磁盘占用检查时再注入 `FloodStage` / `FloodUsedFraction`。本包不另定义默认政策。`Now` 可复用宿主时钟。
- `Record` 原子保存并同步文件及目录，耐久确认后才返回不含载荷的 `Reference`。不同实例共享文件锁，写前回收真正到期的记录并检查真实总量；达到容量或 flood stage 明确失败，不删除未到期结果。
- `Read` 核验引用及完整正文摘要，返回原结果。保留期内记录、值、失败或依据缺失、变化返回 `TEMPORARY_UNAVAILABLE`；正常到期返回 `CAPABILITY_UNSATISFIED`。`Prune` 只按 recorder 管理的期限删除到期记录，正文损坏不能提前结束保留承诺。

公开类型与验证在 `contract.go`，请求捕获在 `collector.go`，本机耐久 adapter 与生命周期在
`file_store.go` / `lifecycle.go`。认证证明由服务边界阻止进入取值结果；本包不接收传输认证头。
