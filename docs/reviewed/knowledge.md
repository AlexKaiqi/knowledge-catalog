# Knowledge

定位：整理稿。能力简介，正文后续梳理。

在固定 commit 上解释或写入 Canonical：身份、Address、Schema、来源、Binding 声明。对象图见 [`core-concepts.md`](core-concepts.md)，不要画进核心架构。消费侧 Serving 把 Snapshot unit 与 `StateLookup` 观察编进同一个 KnowledgeValue。

**独立验收。** Reader 合同 + Writer 合同（PUT/REMOVE、幂等、单仓）。

**不依赖也能说清的失败。** 路径当身份；Workspace 当写目标；PATCH / APPEND。

**明确不是。** 索引、Catalog 正文、动态值当 commit。
