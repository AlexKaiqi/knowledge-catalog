# Snapshot Store

定位：整理稿。能力简介，正文后续梳理。对应核心架构第 2 层。

不可变 commit / ref / CAS / tree。不认识知识对象。

**独立验收。** `snapshot.Store` 合同：读写树、CAS、固定 ref。

**不依赖也能说清的失败。** 无 commit 却报成功；旧 ref 读不到。

**明确不是。** Aspect、`object_id`、SEARCH。
