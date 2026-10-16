# OBJECT-REFACTOR-ORDER-01 · 重构顺序纠正（2026-10-12）

- 状态：此处逐层实施的顺序建议已撤回，未执行。历史修订范围与质量失败记录保留，当前唯一执行路线归 OBJECT-REFACTOR-VERIFY-01 与 object-refactor-plan 的 R0～R3。
- Goal / owner：执行顺序归 object-refactor-plan，边界与依赖原则归 object-refactor-guideline；保持现有合同、发布发现切片及 Snapshot 在途范围。
- 判断 / 选定：原计划先迁 Interfaces/Discovery、P4 再切换基础类型、Reader 与编码，至少需要第二次调用方适配；基础阶段必须倒查上层需求，完成最终公共合同与路径。解除下层反向依赖只做必要局部拆解，不提前重构整个上层。
- 验证范围：`make check-docs`、`make quality`、文档增量复核；只验计划修订，不执行代码迁移、业务或部署验收，不安装依赖、不提交。
- 当时证据与处置：文档图 32 篇 / 143 条关系通过，`git diff --check` 通过；`make quality` 退出码 2，停在其它在途改动的 gofmt 检查（`snapshot/access/update.go`、`snapshot/access/update_test.go`、`snapshot/service/access_http.go`）。当轮未改这些源码或削弱门禁，未取得该轮质量通过；来源改动后需要新的有效证据。这个记录不作为当前路线已验收依据。

## 关闭

2026-10-15 拆分 `TASK.md` 时关闭，未按原条目验收。本条建议当时已撤回，未执行。
