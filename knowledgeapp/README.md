# Typed Application Core

`knowledgeapp` 是 `SERVICE_ARCHITECTURE.md` §1.2 的应用用例边界，不是新的协议层。

当前纵切：

- `ReadExecutor`：固定 Repository/commit 上的对象或 Address READ；
- `SearchExecutor`：固定 Repository/commit 上选择 Snapshot 或已准备的 State 投影；
- `CommitExecutor`：把 typed command identity 与 `writer.CommitIntent` 交给唯一 Writer。

请求中的 Repository、commit、object 和 Address 使用各 owner 包的命名类型。CLI/HTTP 负责
解析 argv/wire DTO、鉴权装配与输出编码；本包不接收 verb、`FlagValue`、`http.Request` 或
provider 私有信封，也不打开 Home、注册 route、选择 authority/retrieval adapter。

Workspace 组合、授权、交付与证据仍由它们现有 owner 执行；迁移纵切不得把这些语义复制进
transport。边界由 `internal/arch` 的 application-core import 守卫和 CLI/HTTP wiring 断言
保持，行为由本包 typed executor 测试及现有 `API-01` 合同共同验证。
