# Outbound hooks

定位：整理稿。能力简介，正文后续梳理。外围能力，不进核心五层。

动作 pre / post 的薄出站。pre 只能机械拒绝；post 失败不回滚已接受的写。

**独立验收。** pre 只能机械拒绝；post 失败不回滚；REPLAYED 不重放。

**不依赖也能说清的失败。** pre 改 ChangeSet；READ 上挂 Hook。

**明确不是。** Gate、Collector。
