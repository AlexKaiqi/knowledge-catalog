# 文档整理稿

这里是对现有 `docs/` 的逐篇重写，**还不是文档图节点**。

`make check-docs` 只把顶层 `docs/*.md` 与 `docs/graph/` 对账。本目录在升格进图、声明 `ownerTopics` 之前，不拥有任何主题，也不能覆盖 `LAYERS.md` / `KNOWLEDGE_CATALOG_DESIGN.md` 的结论。

| 文件 | 状态 |
|---|---|
| [`core-architecture.md`](core-architecture.md) | 核心运行面（1 与 3 骑在 2 上） |
| [`core-concepts.md`](core-concepts.md) | 知识对象图（Entity / Aspect / Binding / Relation）；不与架构图混画 |
| [`walkthrough.md`](walkthrough.md) | 走查：定位 / 边界 / 规范；目录树；从 bundle 入口走的任务 |
| [`dataset.md`](dataset.md) | 已升格进 `COMPOSITION.md` / `TERMINOLOGY.md` / `catalog/`；本稿保留作对照 |
| [`dataset-authorization.md`](dataset-authorization.md) | 已升格进 `PERMISSIONS.md` / `ARCHITECTURE_INVARIANTS.md`；本稿保留作对照 |
| 下列能力篇 | 仅简介；后续逐篇梳理 |

Ingestion control 与 Retriever 的业界对照已先写在顶层
[`INGESTION_RETRIEVAL_RESEARCH.md`](../INGESTION_RETRIEVAL_RESEARCH.md)；本稿升格时不得覆盖投影控制或检索代数 owner。

旧文继续有效，直到对应整理稿升格并改图。

## 可独立验收的能力

| 能力 | 稿 |
|---|---|
| Authentication | [`authentication.md`](authentication.md) |
| Authorization | [`authorization.md`](authorization.md) |
| Snapshot Store | [`snapshot-store.md`](snapshot-store.md) |
| Knowledge | [`knowledge.md`](knowledge.md) |
| Catalog composition | [`catalog-composition.md`](catalog-composition.md) |
| Declarative access | [`declarative-access.md`](declarative-access.md) |
| Retrieval algebra | [`retrieval-algebra.md`](retrieval-algebra.md) |
| Retriever | [`retriever.md`](retriever.md) |
| Ingestion control | [`ingestion-control.md`](ingestion-control.md) |
| Binding observation | [`binding-observation.md`](binding-observation.md) |
| Resource access / collect | [`resource-access.md`](resource-access.md) |
| Merge gates | [`merge-gates.md`](merge-gates.md) |
| Outbound hooks | [`outbound-hooks.md`](outbound-hooks.md) |
| Access evidence | [`access-evidence.md`](access-evidence.md) |
| Host file projection | [`host-file-projection.md`](host-file-projection.md) |
| Durable recovery | [`durable-recovery.md`](durable-recovery.md) |
