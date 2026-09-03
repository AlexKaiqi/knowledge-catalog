# 知识访问可观测性

范围：谁在什么时候访问了哪个固定版本的知识，以及规范 Agent 如何把多次访问和反馈关联成一条可审计 trace。认证提供方不在本协议内。

本文的 trace 是知识访问证据视图，不等于可采样的分布式诊断遥测。系统 metric、日志、distributed trace、健康检查与 SLO 见 [`SYSTEM_OBSERVABILITY.md`](SYSTEM_OBSERVABILITY.md)。

## Goal

记录谁在什么时候访问了哪个固定版本的知识，以及规范 Agent 如何把多次访问和反馈关联成可审计 trace。

## Non-Goals

- 不是可采样的分布式诊断遥测（`SYSTEM_OBSERVABILITY.md`）。
- 认证提供方不在本协议内（文首）。
- 访问证据不是 Canonical，也不改变授权（`O-01`）。

## 硬性约束 / Invariants

- `O-01` access/trace/hitmap 不得写回知识，也不得改变权限或 Canonical。
- `V-01` 证据目标必须是固定 `repository + commit + object/Address`。
- 授权按 `principal` 求值；`onBehalfOf` 是审计事实。
- 写入 fail-closed 追加；查询是时间窗等值过滤与有界页，不是知识 SEARCH。

## 选定方案 / 被否决方案

- 选定：版本化过程账；hitmap 从访问证据派生。
- 否决：用访问次数当知识；把 JSONL 当成第二种知识 Store；把本文件的身份 JSON 示例当成传输合同（传输头见 `SERVICE_ARCHITECTURE.md`）。

## 接口契约 / 状态机

协议口是 Recorder（fail-closed 追加）与 AccessLog（有界页查询）。目标必须是固定 `repository + commit + object/Address`。本机 JSONL 只是一种 adapter。`principal` / `onBehalfOf` 语义由本文与 `PERMISSIONS.md` 拥有；传输头不在本文。字段、页与 CLI/HTTP 形状以 `observability/README.md` 为准。

## 身份

每次调用携带认证边界已经建立的身份：`principal` 是实际执行者，必填；`onBehalfOf` 是其代理的用户，可选且必须经过认证器验证。用户直接访问时 `principal` 为用户，省略 `onBehalfOf`。授权按 `principal` 求值；`onBehalfOf` 是审计事实，不参与授权交集。含义不随认证器替换而改变。传输头与自报身份拒绝规则见 `PERMISSIONS.md` 与 `SERVICE_ARCHITECTURE.md`。

## 访问账

知识消费与 `file.read` 等 semantic action 完成后，应用服务经 Recorder 追加一条 access 事件。成功、失败和拒绝都记。Bound State 实际 hydrate 时证据同时保留声明 basis 与 observation basis；VFS 只记固定文件坐标。`evidenceId` 由 Recorder 生成，调用方不得提供；ack 表示已越过耐久边界。覆盖率对账用 `evidenceId`，不用可能重复的 `requestId`。

访问账是过程证据，不进成员 Repository，不是 provenance，也不改变 Canonical。记录失败会使成功的 facade 响应失败。字段与页合同以 `observability/README.md` 为准。

## 证据库：写入与访问

访问证据有自己的 Store 合同，但不是 `snapshot.Store`、不是 Knowledge Repository，也不是检索 projection。Catalog 不登记它；SEARCH 不发现它。本机 JSONL 只是 adapter。

写入：一次 semantic action 一条事件（可含多个固定目标）。已 ack 的事件不可改、不可按 `requestId` 去重。写口不鉴 `audit.read`。不写知识正文、凭证或模型隐式推理。

访问：查询走 Operations 的 `audit.read`。三种读——按 `evidenceId` 点查、时间窗加等值过滤的有界页、以及 trace/hitmap 派生折叠——形状见 `observability/`。`limit=0` 表示默认页，不是全量导出。查询不是知识 SEARCH。

公开入口是 Operations 上的 audit 面，不是本文复制的 CLI 开关表。

## Agent trace 与反馈

规范 Agent 用 trace/span 关联同一任务中的 KC 调用。传输坐标由服务架构拥有。反馈只接受已经存在知识访问的 trace，避免孤立反馈。完整 trace 是知识系统边界内的调用证据，不保存 chain-of-thought。绕过应用服务直读 Git 或索引时，KC 无法观察逐条访问；`kcfs` 必须走同一 Recorder 接缝。

## 系统性检索证据模型

检索观测不是一种日志覆盖所有阶段。职责分离：

| 原始账 | 覆盖 |
|---|---|
| access | 身份、授权、固定知识 basis、Canonical READ |
| retrieval | SEARCH / 一跳 RELATION 的逻辑请求与候选窗 |
| refine | 可选语义 filter/rerank 的模型可见输入输出 |
| feedback | Agent 答案、引用、用户确认或纠正 |

hitmap 与 training sample 是可重建派生视图，不是第三种权威。SEARCH/RELATION 成功响应返回 retrieval evidence id；下游 refine 失败不得涂改已 ack 的 retrieval。精确 READ 只归 access。查询口在 Operations；类型在 `observability/`。

未执行的计划不得伪装成观测。未来增加 lexical/dense 等 stage 时，在同一 retrieval 事件上加版本化 source facts。

## Semantic Refine 与训练闭环

进入语义 Provider 的调用在 access 落盘后追加独立 refine 证据。它记录精确模型输入（投影值与 digest），不是 Canonical 副本，仍可能含业务敏感内容，只有 `audit.read` 可查。明确不保存 API key、transport body、未投影字段和隐式推理。refine append 失败时不得向调用方返回成功 rerank。

监督标签：Agent 自评默认不可训练；用户/评审的接受或纠正才形成可训练样本。训练流水线在墙外，不回写知识仓。

## Hitmap

hitmap 从成功的 `ALLOW + RESOLVED` 访问事件派生，分组键是 `repository + commit + object + Address`。删除原始账后可重建为空。它从不反向影响授权、搜索排序或知识内容。
