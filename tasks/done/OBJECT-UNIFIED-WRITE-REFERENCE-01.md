# OBJECT-UNIFIED-WRITE-REFERENCE-01 · 读写共用 URN＋locator（2026-10-13）

- [x] 设计文档完成：Guideline 的创建、更新、删除与读取已统一使用对象 URN＋locator，PUT／REMOVE 主示例及粒度表已改；现有分散参数只记兼容适配。目录 diff／commit 已从 Object 自身清单移出，计划同步统一引用与入口边界验收。
- 范围与出口：只改 Guideline、计划及本条，不改 Go、现有 CLI／API、存储或写入原子性，不提交；URN 字面编码及真实参数落点沿公开合同实现，本轮示意不冒充可执行命令。只选定既有完整维护目标的统一引用，不借此增加集合／深字段删除语义。
- 质量处置：此前将现有参数及 Address 能力限制写成未来统一入口的形状，判定为现状收窄目标；主合同改为统一引用、服务按维护模型计算变更，旧参数和限制移入兼容说明。集合／深字段删除是否扩为新的公开操作范围另行选定，不用旧 Go 类型自动否决，也不以统一编码声称支持。
- 验证：统一引用、三种维护目标、兼容及未选定删除范围经独立复核通过；已重读 owner／图，`make check-docs`（33 文档／171 关系）及本轮空白检查通过。`make quality` 因共享源码 `snapshot/service/file_view.go:79` 的 `mountOwning` 未使用而失败（日志 `/tmp/kc-object-unified-write-reference-quality-20261013.log`）；保留在途 Snapshot 修改并记录阻点，源码处置后重跑，不绕过静态检查、不记整仓质量或统一 CLI 实现通过。
- 目录入口更正：此前以“Object CLI 适配”将目录 diff／commit 留在本模块表，混淆了能力使用与所有权。当前共享修订已移出该表，本轮补清应用知识发布流程、Object Writer 校验／写入保证及 Snapshot 文件／版本提交的责任，区分已有对象两版 Reader 对照与当前目录 diff。迁移触碰目录流程时按应用任务和兼容验证，不因现有函数位置固化所有权。
- 本轮出口：对象差异 API、目录实际执行路径及模块职责经独立复核，owner／图重读；`make check-docs`（33 文档／172 关系）、`make quality`（退出 0，日志 `/tmp/kc-object-directory-task-scope-quality-20261013.log`）及本轮空白检查通过。先前共享静态检查阻点本轮已解除；只确认文档范围完成，URN／locator 真实写入入口、帮助内容及对象差异 CLI 仍待实现／选定。
