# OBJECT-WRITE-BOUNDARY-01 · Object 入口归属与写入粒度（2026-10-13）

- [x] 文档修正完成：Guideline 已区分 Object 核心操作、目录适配与恢复辅助；写清 PUT／REMOVE 的整体值、Aspect、Member 粒度和限制，移出 Binding 专属解释，resolve 降为可选诊断。
- 范围与出口：只改 Guideline、计划的对应验收及本条，不改命令、API 或运行实现；以身份、表示、固定读写及来源记录判断归属，不以包位置或不外呼为依据。独立核对公开合同，重读 owner／图，文档及质量门禁全绿后只记文档完成。
- 质量处置：此前“纯声明解释即属 Object”的判断已撤回。`object/README.md` 首段与 `object/reader/binding.go` 仍收纳 LiveAccess 专属解析，判定为待迁移的职责混合；本轮在指南和计划明确目标，触碰声明解析迁移时移动唯一实现及调用方、同步 README／Conformance，不新增第二份解析器。
- 验证：参数、三粒度与删除范围独立核对通过；修正“必须先读再 PUT”的过度约束，明确完整新值可直接提交。已重读 owner／直接图关系，`make check-docs`（33 文档／171 关系）、最终 `make quality`（退出 0，日志 `/tmp/kc-object-write-boundary-quality-final-20261013.log`）及本轮空白检查通过；未改代码、执行示例写入或确认新 URN／locator 功能实现。
