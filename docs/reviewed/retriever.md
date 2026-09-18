# Retriever

定位：整理稿。能力简介，正文后续梳理。对应核心架构第 4 层（Search Index 的 provider 面）。
业界对照见 [`../INGESTION_RETRIEVAL_RESEARCH.md`](../INGESTION_RETRIEVAL_RESEARCH.md)；
查询代数仍由 [`../RETRIEVAL.md`](../RETRIEVAL.md) 拥有。本稿升格前不拥有主题。

某个 provider 对 AccessSpec 的可丢投影。换引擎仍只出候选，不充当 Canonical。

**独立验收。** 同一 RetrievalPlan，换 OpenSearch 等仍只出候选。

**不依赖也能说清的失败。** BUILDING 当空成功；物理 stored 当知识。

**明确不是。** Canonical、Writer、Catalog。
