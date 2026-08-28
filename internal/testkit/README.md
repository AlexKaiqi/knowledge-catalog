# internal/testkit/

跨包测试装置与 conformance runner，只供测试使用。

| 文件 | 负责 |
|---|---|
| `testkit.go` / `memory_store.go` | 只供测试的内存 Store 与 Reader/Writer fixture；不是可配置 adapter |
| `contract.go` | T1–T12 Repository/Snapshot conformance |
| `writer_contract.go` | Writer 边界契约 |
| `gitea.go` | 进程级 Gitea 测试服务装置；使用包须在 `TestMain` 调 `StopGitea`，不得遗留容器 |

具体适配器合同归具体适配器包：Dolt 和 Gitea 分别运行同一 Repository/Writer conformance。通用知识包测试不得 import 具体 adapter，也不应隐式启动它们。
