# Declarative access

定位：整理稿。能力简介，正文后续梳理。

Schema 只声明逻辑 `text` / `filter` / `sort`，编译成 AccessSpec。它说明「哪些字段可被怎样问」，不说明索引怎么存。

**独立验收。** `DESCRIBE_SCHEMA`；拒绝 provider / stored / key。

**不依赖也能说清的失败。** Schema 里出现物理引擎词；GRANT 当 FTS。

**明确不是。** 索引 mapping、RetrievalPlan 算子、投影维护。
