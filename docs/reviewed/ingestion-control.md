# Ingestion control

定位：整理稿。能力简介，正文后续梳理。对应核心架构第 3 层。
业界对照与名词映射见 [`../INGESTION_RETRIEVAL_RESEARCH.md`](../INGESTION_RETRIEVAL_RESEARCH.md)；
算法仍由 [`../PROJECTION_CONTROLLER.md`](../PROJECTION_CONTROLLER.md) 拥有。本稿升格前不拥有主题。

感知 Snapshot / observation 变化，对账并调整派生投影。正确性靠对账，不靠把通知做可靠。
它不是写面：不接收 ChangeSet，不跑 Writer 校验，投影失败不回滚权威。外部直推 published
ref 时，慢路径 Desire(HEAD)，按 ② 解释 frontmatter 再 Ensure。

**独立验收。** 控制器：diff → 受影响对象 → Ensure → READY；多消费者互不阻塞。

**不依赖也能说清的失败。** 消费请求同步 build；投影失败回滚 commit；缺变化识别就全仓扫。

**明确不是。** SEARCH 算子、Binding 语义、权威存储、Writer / 采集写面。
