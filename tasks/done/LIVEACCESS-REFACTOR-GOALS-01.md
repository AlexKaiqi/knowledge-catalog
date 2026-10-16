# LIVEACCESS-REFACTOR-GOALS-01 · LiveAccess 重构目标与约束（2026-10-13）

- [x] 已删除重复的失败目标、未选定的对象整体拒绝政策及版本/Schema/搜索泛套话；插件篇四处继承的整体拒绝表述同步清理，未补替代政策。
- 本轮删除验证：LiveAccess 正文含标题 836 字符，12 用例原编号/标题保留；相关 owner/图已重读，独立检查传播遗漏。`make check-docs`、`make quality`（退出 0）及增量空白通过，日志 `/tmp/kc-liveaccess-remove-invented-policy-quality-20261013.log`。未改 Go、权限/搜索 owner 的既有正式合同或守卫；单值失败继续由 Goal 3 定义。
- 删除验证：正文含标题 927 字符，已重读 owner/图，文档与质量检查、增量空白检查通过；日志 `/tmp/kc-liveaccess-remove-write-aside-quality-20261013.log`。仅删文档一句，未改 Go 或公开合同。
- 本轮证据：LiveAccess 正文含标题 955 字符，12 用例原 ID/标题保留；service owner 和阅读入口同步，原值/记录保留期限匹配，既有 READ 版本与观察依据继续返回，凭证不进入证据。独立审查、owner/图重读、最终 `make check-docs`、`make quality`（退出 0）及增量空白通过；日志 `/tmp/kc-liveaccess-audit-goal-quality-20261013.log`。仅完成设计目标，实际值的保留关联与审计回读仍由下述实施出口补齐。
- Goal / owner：LiveAccess owner 的插件目标、句柄取值、单值失败与身份传递；已按 docs/graph 读取 ownerTopics/直接关系，相关授权与对象能力合同沿 permissions/object-interface。Non-Goals：仅文档与阅读入口/图登记，不改 Go、公开形状、场景、测试或运行配置，不实施插件，不提交。
- 不变量 / 取舍：保留 V-01、AUTH-01～03、OI-01～05、API-01；选定独立目录、真实插件、透明取值、单值失败隔离和适用身份自动传递，只规定目标/约束；否决实现路线、Collector 混入主线、造新能力名或未经选定的 Handle/错误 DTO。
- 质量出口：`liveaccess/service/hydrate.go` 当前单值错误直接上返，与用户新定的局部失败目标有差距；判定为待实现目标而非文档完成即可验收，LiveAccess 重构时按新目标选定公开错误表达并补局部失败回归，受影响合同与工程闸全绿，不用 skip 或迎合现状。对象整体拒绝不是用户选定目标，不据历史记录新增该类验收要求。已有生产保留/凭证目标/直接 access 核验债保留原专项出口。
- 访问证据 owner / 出口：通用访问记录及独立审计归 service，实际值及原观察保留归 LiveAccess；已读 docs/graph 端点、AccessEvent/AccessLog、观察与服务公开合同。现有 AccessEvent 有身份/时间/RequestID/固定目标及 basis，无实际值，判定为独立审计尚不能据此完整复核 LiveAccess 当次数据；重构接入时补实际值保留关联及独立审计回读，验源已更新仍返原值、单值失败说明、重启恢复、留证失败与保留期限。仅补设计目标，不预定存储位置、新 DTO 或查询字段，不修改普通 READ 既有版本/观察依据。
- 文档模板债：已由 DOC-TASK-SIMPLIFY-01 按用户约定取消固定模板并清理空 Non-Goals 段。
- 验证：独立复核目标与拒绝边界，重读 owner/图，核对全文字数与稳定用例，运行文档、质量和增量空白检查后才勾选；结果只证明文档，不证明透明取值/局部失败/身份传递已实现。
- 清理验证：最终可见正文含标题 704 字符，12 用例 ID/标题不变，owner/图已重读；`make check-docs`、`make quality`（退出 0）及本轮空白检查通过，日志 `/tmp/kc-liveaccess-text-cleanup-quality-20261013.log`。行为约束仅移动/去重，未改 Go 或协议。
- 交付结果：可见正文含标题 882 字符；独立审查通过，12 个原用例 ID/标题保留。阅读入口及图标题同步，源侧角色边界主题归术语 owner；插件例子同步局部失败目标并保留对象授权/搜索完整性/交付拒绝边界。`make check-docs`、`make quality`（退出 0）与本轮空白检查通过，日志 `/tmp/kc-liveaccess-goals-quality-20261013.log`。未改 Go 或运行配置，现有 hydrate/README 的错误上返行为仍按上述实施出口待补齐，未把文档验收写成插件实现验收。
