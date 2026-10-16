# OBJECT-LOCATOR-EXAMPLES-01 · 内容定位符写法与例子（2026-10-13）

- [x] 示例文档完成：Guideline 的 Address 章节已补 locator 具体写法、对象内容例子、选取结果及 JSON Pointer／URI 两层转义；明确对象根、Member、深字段、数组位置与字面键名。
- 范围与出口：不改身份／访问语境、Affordance、代码、CLI／API、其它设计或图关系；按 RFC 6901 独立核对示例，重读 owner／图，文档与质量门禁通过后只标记文档完成。
- 验证：示例及转义独立复核通过，明确对象按 Aspects、columns 按 Members 维护，未从 JSON 外形推导写粒度；编辑前快照确认仅扩写 locator 示例。`make check-docs`（33 文档／171 关系）、`make quality`（退出 0，日志 `/tmp/kc-object-locator-examples-quality-20261013.log`）及本轮空白检查通过；未实现新读取入口。
