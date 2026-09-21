# Typed Application Core

`knowledgeapp` 是 `SERVICE_ARCHITECTURE.md` §1.2 的应用用例边界，不是新的协议层。

当前纵切：

- `ReadExecutor`：固定 Repository/commit 上的对象或 Address READ；
- `SearchExecutor`：固定 Repository/commit 上选择 Snapshot 或已准备的 State 投影；
- `CommitExecutor`：把 typed command identity 与 `writer.CommitIntent` 交给唯一 Writer。
- `DatasetPublisher`：授权 → 冻结候选 → 必要派生能力准备 → 原子接受新发布；失败保留旧版。
- `DatasetReadExecutor`：授权 → 固定消费范围 → 逻辑读取 → 交付；声明与正文使用同一范围。
- `DatasetSearchExecutor`：授权 → 固定消费范围 → 有界多仓查询 → 范围内回读 → 交付；范围过滤发生在缓存、权威与 State 正文读取之前。
- `DatasetRelationsExecutor`：按固定 Dataset 成员查询关系存储仓，端点可以在另一成员仓；范围过滤先于正文回读，游标绑定各仓版本、查询与文件范围。

请求中的 Repository、commit、object 和 Address 使用各 owner 包的命名类型。CLI/HTTP 负责
解析 argv/wire DTO、鉴权装配与输出编码；本包不接收 verb、`FlagValue`、`http.Request` 或
provider 私有信封，也不打开 Home、注册 route、选择 authority/retrieval adapter。

组合、授权、交付与证据规则仍由各 owner 定义；应用核心通过注入端口决定调用顺序，不在
transport 复制规则。边界由 `internal/arch` 的 application-core import 守卫和 CLI/HTTP wiring 断言
保持，行为由本包 typed executor 测试及现有 `API-01` 合同共同验证。
