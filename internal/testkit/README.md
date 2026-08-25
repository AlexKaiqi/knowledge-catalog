# internal/testkit/

跨包测试装置与 conformance runner，只供测试使用。

| 文件 | 负责 |
|---|---|
| `testkit.go` | FileGit/Reader/Writer 基础 fixture |
| `contract.go` | T1–T12 Repository/Snapshot conformance |
| `writer_contract.go` | Writer 边界契约 |
| `gitea.go` | 进程级 Gitea 测试服务装置；使用包须在 `TestMain` 调 `StopGitea`，不得遗留容器 |

具体适配器合同归具体适配器包：FileGit reference contract 在 `knowledge/`，Dolt 在 `snapshot/dolt/`，Gitea 在 `snapshot/gitea/`。通用知识包测试不应隐式启动 Dolt 或 Gitea。
