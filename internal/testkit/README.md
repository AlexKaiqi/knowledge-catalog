# internal/testkit/

跨包测试装置与 conformance runner，只供测试使用。

| 文件 | 负责 |
|---|---|
| `testkit.go` / `memory_store.go` | 只供测试的内存 Store 与 Reader/Writer fixture；不是可配置 adapter |
| `contract.go` | T1–T12 Repository/Snapshot conformance |
| `writer_contract.go` | Writer 边界契约 |
| `json_fixture.go` | 按调用方公开 wire 类型严格检查测试 JSON，并执行调用方注入的语义校验；不拥有业务字段 |

具体适配器合同归具体适配器包；lakeFS 夹具运行 Repository/Writer conformance，真实部署另行验收。
通用对象包测试不得 import 具体 adapter，也不应隐式启动它们。

真实 lakeFS 场景夹具的 `SnapshotStamps` 只接纳本次创建且存储命名空间一致的仓，保存分支头与仓属性；顺序恢复清除新分支和未提交写入，保留固定提交及历史。它不复制仓、不重放发布，不可用于共享部署权威或并发隔离；普通进程内夹具仍使用物理仓分叉。
