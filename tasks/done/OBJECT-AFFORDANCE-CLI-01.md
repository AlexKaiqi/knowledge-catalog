# OBJECT-AFFORDANCE-CLI-01 · 以 CLI 入口说明对象能力（2026-10-13）

- [x] 文档修正完成：Affordance 章节已改为真实 CLI 命令与一句用途，移除抽象任务分类；明确 diff、Schema 查询、回执查询的实际保证，URN／深 locator 单列为待实现能力。
- 范围与出口：仅 Guideline 本章节及本条，不改 CLI、公开形状、其它设计或图关系；核对真实 help、包 README 与 SURFACE，独立复核并确认其它章节不变，文档及质量门禁通过后只标记文档完成。
- 验证：真实 `kc help` 正常返回，命令表与模块帮助、SURFACE 对照，独立复核通过；编辑前快照确认 Guideline 其它章节一致。`make check-docs`（33 文档／171 关系）、`make quality`（退出 0，日志 `/tmp/kc-object-affordance-cli-quality-20261013.log`）及本轮空白检查通过；未实现 URN／深层定位或变更运行入口。
- 范围纠正：该轮仅核对入口存在与描述，却将对象相关的外部职责混入 Object 清单；所有权判断错误已由 OBJECT-AFFORDANCE-SCOPE-01 修正，并升级为计划的完成合同。工程门禁通过不替代语义归属核验。
