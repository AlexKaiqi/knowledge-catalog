# DEBT-WRITER-CROSS-REPO-SCHEMA-01 · Writer 尚未承接跨仓 Schema 采用

## 债

- 位置：`object/writer/schema.go`、系统仓建模指南与公开教学材料。
- 信号：目标仓只引用系统仓的 readme Schema 时，目录发布返回 SCHEMA_REVISION_UNRESOLVED；
  Writer README 要求目标 commit 可解析，而设计要求可以按 ID 跨仓复用并固定版本。
- 判定：能力缺口。跨仓读取依赖固定与 Writer 采用校验相关，但不是同一个入口。
- 处置：公开练习显式导入同身份原文；共享导入夹具用守卫核对系统出版物。
  后续为 Writer 提供有范围、授权和固定版本的 Schema 解析，不能扫描任意 HEAD 冒充固定依赖。
- 触发条件：实现跨仓 Schema 采用，或下一次修改 Writer 的 Schema 解析路径前。

## 核实（2026-10-21）

债仍然成立。`object/writer/schema.go` 仍保留 `kernel.ErrSchemaRevisionUnresolved` 这条失败路径：
目标仓只引用系统仓的 readme Schema 时，目录发布在该处失败，因为 Writer 要求目标 commit 内可解析。
设计允许按 ID 跨仓复用并固定版本，所以这是能力缺口而非文档不一致。

性质是新功能：需要为 Writer 提供有范围、带授权、固定版本的 Schema 解析，并让回执保留采用依据、
重试不改读最新。本轮不改。

去向：与 `OBJECT-VERSIONING-01` 的解释依赖固定依据一并设计，不另造一套版本机制；
触发条件不变——实现跨仓 Schema 采用，或下一次修改 Writer 的 Schema 解析路径前。

## 目标

接入方按设计复用外仓 Schema，发布时不必在目标仓复制同构定义。

## 依据

- [对象](../../doc/design/object.md)：Schema 复用与固定版本。
- [Dataset](../../doc/design/dataset.md)：解释依赖与交付范围。

## 验收

覆盖唯一外仓 Schema、固定旧版、无权、歧义、缺失和并发变化；回执保留采用依据；
重试不改读最新；受影响 Writer、读回与架构合同及质量门禁全绿。

## 前置

与 `OBJECT-VERSIONING-01` 协调解释依赖的固定依据，不另造一套版本机制。

## 不做

不因公开教程需要而扩大对象协议或隐式复制源仓。

## 关闭

2026-10-21 并入 `OBJECT-VERSIONING-01`：位置、信号与剩余验收在该任务的“并入”一节继续推进。
本条已有的核实、交付与守卫保留为历史，不代表验收通过。

2026-10-22 用户裁定关闭，不再作为能力缺口：实例采用外仓 Schema 属边界场景，不是核心能力，不实现。
设计与公开文档已收窄为同仓口径，复用路径是把 Schema 原文导入目标仓并保持原身份。
索引声明的字段定位符可用仓限定指名外仓 Schema，那是另一条已实现路径，不受影响。
