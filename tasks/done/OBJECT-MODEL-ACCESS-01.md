# OBJECT-MODEL-ACCESS-01 · 首先能访问对象模型概念（2026-10-13）

- [x] 根帮助设计文档完成：Guideline 的 Goal、能力清单及 CLI 第一项已改为 `kc help` 本身直接解释 object_id、Aspect、Member、Address、Relation、联系、基础能力与小例；详细参数沿既有叶子帮助，系统导读和概念对象留作深入阅读；计划同步仅沿根帮助的零知识走读验收。
- 范围与出口：只改 Guideline、计划及本条，不实现帮助、不新增帮助主题、业务命令、概念标识、API 或出版实现；Object 提供模型与固定读取，完整系统出版仍归 SystemRepo。独立核对出版源／owner／图，文档与质量门禁全绿后只记文档完成。
- 质量登记：`systemrepo/content/concepts/object/definition.md` 将业务对象与知识表示混作“知识载体”，`concepts/address/definition.md` 尚未说明新 URN／locator 目标；判定为模型说明需随重构同步的出版内容债，Object 对应语义／访问切片落地时按 owner 修订出版源、同源说明及正式出版回归，不在本文虚构尚未选定的 URN 编码或已部署新功能。
- 验证：概念身份、CLI 参数、object_id 说明位置和模块／出版职责经独立复核通过；重读 owner／图，`make check-docs`（33 文档／171 关系）及本轮空白检查通过。最终 `make quality` 因共享工作区的 `snapshot/dataset/publication_source_validation_test.go` 需 gofmt 而失败（日志 `/tmp/kc-object-model-access-quality-final-20261013.log`）；记录阻点并保留在途源码，待其格式修正后重跑，未绕过门禁、未记整仓质量或部署通过。
- 入口纠正：直接读取系统概念仍要求预先知道仓和标识，不能作为零知识起点；已改为 root help → 模型静态帮助 → 叶帮助 → 可选的系统定义读取。现有 CLI owner 已要求模块概念帮助静态提供，当前 rootHelp／helpFor 尚缺 Object 模型主题；实现时按仅知道 `kc help`、无会话／服务配置且无业务请求的独立走读取证，不以文档选定冒充已实现。
- 本轮出口：真实 `kc help` 正常返回且无模型导航，`kc help object` 返回未知主题，确认当前入口缺口；独立职责复核、owner／图重读、`make check-docs`（33 文档／171 关系）、`make quality`（退出 0，日志 `/tmp/kc-object-zero-knowledge-help-quality-20261013.log`）及空白检查通过。此前共享格式阻点本轮已解除；仅标记设计文档完成，模型帮助与 URN／locator 功能尚未实现。
- 根入口更正：新增 `kc help object` 为必经页的设计撤回；根帮助直接承担模型概览，叶子帮助才展开操作参数。不把“找到下一页”冒充已经理解模型，也不把根帮助扩成全部参数手册；本轮独立复核通过，当前待补是根帮助内容。
- 根帮助出口：独立充分性复核、owner／图重读、`make check-docs`（33 文档／171 关系）、`make quality`（退出 0，日志 `/tmp/kc-object-root-help-quality-20261013.log`）及空白检查通过；只完成根帮助目标文档，CLI 内容未改。
