# 外部资源：访问句柄与知识采集

日期：2026-08-24

范围：外部系统仍持有运行态或领域权威时，Agent 如何访问，以及外部变化如何进入知识 Repository。

---

## 0. 只有两个领域概念

```text
ResourceDescriptor ──→ Agent 访问外部资源

Collector ───────────→ Writer COMMIT / APPEND ──→ 更新知识
```

除此之外：

- 身份、登录、授权、Agent/session 信息和调用 trace 是全系统统一能力，不属于本协议的局部概念。
- 运行环境、Repo 监听、部署、注册和凭证管理是托管基础设施，不是知识对象。
- Connector、Provider、Driver、Shape、Face、Request、Result 等不进入 Agent 的核心术语表；具体访问协议写在 Descriptor 文件里即可。

---

## 1. ResourceDescriptor：一个自包含的访问句柄

ResourceDescriptor 是知识 Repository 中的一份普通、可版本化文件。Agent 读取这一份文件，就能知道：

- 资源是什么、适合回答什么问题；
- 通过哪个已注册运行实现访问；
- 可以调用哪些操作，参数怎么传；
- 会返回哪些信息，分页、窗口、完整性和限制是什么。

示意：

```yaml
object_id: resource/traces/payment-api
kind: ResourceDescriptor

description: Payment API 的生产 Trace，可按窗口读取或按 traceId 检索。
runtime: observability-prod
protocol: resource-access/v1

access:
  status:
    call: stat
    returns: [head, retention, availability]
  window:
    call: window
    input: {from: timestamp, to: timestamp, cursor: optional-string}
    returns: {records: trace-span[], nextCursor: optional-string, cut: string}
  lookup:
    call: lookup
    input: {traceId: string}
    returns: {records: trace-span[], cut: string}
```

字段不是另一套平台枚举。不同资源可以声明 `status`、`window`、`search`、`readRange` 或其他协议；运行实现按 Descriptor 中声明的协议处理。

“自包含”表示 Agent 不需要再追第二份 Binding、能力说明或 Skill 才能构造访问；不表示文件包含 endpoint、token、任意 URL 或源侧秘密。Descriptor 是句柄，不是凭证。

### 1.1 VFS 与 Skill

VFS 直接把 ResourceDescriptor 当普通文件暴露：

```text
READ descriptor file
→ 调用全系统统一的 resource access
→ runtime 按 descriptor 执行
```

读取 Descriptor 本身不触源，也不返回 live 内容。大日志不能伪装成一次普通 `read(file)`；它应在 Descriptor 中声明窗口、游标、检索或范围访问。

Skill 只是通用使用说明，例如“如何发现 ResourceDescriptor、如何调用 resource access”。每个资源的具体协议已经在 Descriptor 内，不需要再生成一份资源专属 Skill。

### 1.2 身份与 trace

resource access 复用全系统的身份和执行链路：先登录，平台得到用户身份；由 Agent 执行时，平台同时知道 Agent、session、delegation 和 request。Descriptor 不再定义一套身份结构。

每次调用至少由全局 Agent trace 记录：

```text
用户 / Agent / session / request
+ pinned descriptor version
+ 实际运行版本
+ 调用参数
+ 外部资源坐标或 cut
+ 结果摘要、错误与耗时
```

payload 可以按策略脱敏或不落 trace，但不能丢失 Descriptor 版本、资源坐标和结果摘要。这样引入 VFS 不会牺牲闭环。

---

## 2. Collector：把外部变化更新成知识

Collector 是第二个、也是唯一另需命名的领域角色：它读取外部当前态或事件，把它们翻译为现有 Writer 输入。

```text
外部当前态 ──→ Collector ──→ ChangeSet ──→ Writer COMMIT
外部事件   ──→ Collector ──→ Entries   ──→ Writer APPEND
```

Collector 不新增 Write Surface，不直写 git。STATE 对账可以复用现有 `connector.Preview`：

- `patch` 只 PUT 本次 Desired，不推断删除；
- `reconcile` 只在 `Observed ∩ Scope` 内产生 REMOVE；
- Desired 超 Scope 整批拒绝；
- 空预览不提交；
- 写入仍由 Writer 处理 CAS、幂等、schema 和 provenance。

访问和采集互不隐含：Agent 调一次 Descriptor 不会自动写知识；Collector 更新知识也不要求 Agent 参与。

---

## 3. 共建 Repo 与运行环境

业务方与平台共建一个独立的 integration repo。它是交付和运维单元，不是 Knowledge Repository，也不是 Workspace 成员。Repo 内负责：

- 开发协议与实现代码；
- 测试和兼容性；
- owner、维护与版本策略；
- 如何构建、运行、健康检查和升级；
- 可选 Collector 实现。

平台提供统一运行环境：

```text
integration repo 发生变化
→ runtime 观察目标 ref
→ 构建 / 验证 / 运行
→ 注册或更新可用运行实现
```

运行环境统一处理身份接入、凭证、网络、隔离、限流、审计和 trace。业务负责源语义、源权限、数据正确性和维护；平台可以托管运行，但不夺走业务责任。

要把某项运行能力交给 Agent，只需经 Writer 把对应 ResourceDescriptor 写进知识 Repository。Descriptor 指向已注册运行实现；Catalog 不解释 Descriptor，也不触发运行。

---

## 4. 当前实现边界

已有：

- 知识文件可经 Writer COMMIT，随 Workspace 固定版本供 Agent 读取；
- `connector.Preview` 提供 Collector 的 Address Scope 对账；
- Writer COMMIT / APPEND 与 Stream cursor 已有；
- DSH 插件提供 `resource` 工具：它先从当前 pinned Workspace 读取 Descriptor，再把用户、Agent preset、session、request 和 Descriptor 的 Repository/commit 一起交给平台访问服务；模型不能传 endpoint、凭证或伪造身份；
- 数仓 scene 的 Integration Host 是可执行参考：观察业务 integration Git Repo，验证并激活固定 generation，按计划运行 Collector，并实现 `resource-access/v1` 与访问 trace；
- Payment API 的真实 DSH 多角色验收已经覆盖：开发集成 → 发布 Descriptor → 自动首采 → 消费实时 trace → 外部源变化 → 自动更新知识 → 再消费与审计。

仍未进入通用协议根：

- 面向生产的多租户 resource access 托管服务、凭证保险库和网络隔离；
- integration repo 的生产构建、签名、发布、扩缩容和升级平台；
- Loki、OTel 等具体外部资源适配器。

这些能力属于平台运行基础设施，不应塞进 `kc` 或 Catalog 协议。当前 `kc stream --workspace` 仍只读取已经 APPEND 的 KC Stream；live Trace 由 DSH `resource` 工具按 Descriptor 调运行服务，只有 Collector 显式 COMMIT/APPEND 后才成为知识。
