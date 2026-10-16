# OBJECT-AFFORDANCE-SCOPE-01 · 按 Object 所有权收口可供性（2026-10-13）

- [x] 范围修正文档完成：清单已删除 Discovery、LiveAccess、Dataset、Governance 与仓级版本管理入口，保留 Object 身份、固定内容／Schema／来源声明、对象写入及历史结果；计划将逐项所有权与基础独立采用纳入完成合同，避免继续按“操作对象”或 CLI 分组判断归属。
- 范围与验证：仅 Guideline 本章节、计划完成观察及本条；不改代码、命令／API、其它 owner 或图关系。沿架构 owner 与 Object／Reader／Writer／Service 合同独立核对，确认其它 Guideline 章节不变，文档及质量门禁通过后只记文档完成。
- 结果：12 项按能力所有权独立复核通过，Schema／Binding 固定声明解释与外部执行分开，Writer 既有候选写入合同保留；Guideline 其它章节与编辑前快照一致。`make check-docs`（33 文档／171 关系）、`make quality`（退出 0，日志 `/tmp/kc-object-affordance-scope-quality-20261013.log`）及本轮空白检查通过。仅完成文档范围修正，未改运行行为。
- 归属更正：上述 Binding 判断撤回；只读的专属解释也不属于基础 Object，已由 OBJECT-WRITE-BOUNDARY-01 从清单移出，固定声明读取与 LiveAccess 语义解释分别说明。历史工程检查不作为该归属判断正确的证据。
