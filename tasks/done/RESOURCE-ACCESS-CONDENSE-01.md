# RESOURCE-ACCESS-CONDENSE-01 · 外部资源访问插件重构合同（2026-10-13）

- [x] 表达修正完成：Non-Goals 直接说明请求/返回格式兼容、随服务构建及启动登记、采集仍走写入入口、通知只提示重查、事件历史/订阅另行设计；没有承诺通知路由不改。最终可见正文含标题 999 字符，12 用例原编号/标题不变，owner/图已重读，文档与质量检查及本轮空白检查通过；日志 `/tmp/kc-resource-access-plain-language-quality-20261013.log`。仅修表达，未改实现。
- Goal / owner：资源访问篇拥有 LiveAccess 插件访问接入与观察保证；共同登记/选用/生命周期沿插件扩展篇。已重读 docs/graph 的 ownerTopics、直接 Relation 和现有装配/调用公开接缝。Non-Goals：仅改文档及图中标题，不改行为、Go、协议、场景、主题所有权或其它任务，不提交。
- 不变量 / 接口：保持 V-01、W-01/W-02、AUTH-01～03、PC-01、API-01；公开形状仍沿 LiveAccess README、StateLookup、ObservationBasis 及既有兼容合同。选定合并重复解释，否决删用例 ID、改变保证或把待实现写成已有能力。
- 债项出口：`liveaccess/service` 尚缺登记上下文与目标目录，首次插件接入补静态入口、受限 typed 登记、实例私有处理器、资源归属及选择反例；`invocation/verbs_resource.go` 直接 access 核验/证据缺口在访问服务改接时统一并复跑已登记回归；latest-only 保留夹具不证明生产能力，保留承诺上线前明确介质、容量、期限、备份与恢复；`liveaccess/state_lookup.go` 证明透传不证明目标信任，出站装配改变前落实权限 owner 的目标/重定向政策并验 U7。
- 首轮压缩证据：非空白源码 1000 字符，可见文本 915 字符，五段与用例提取、`make check-docs`、`make quality` 通过；日志 `/tmp/kc-resource-access-condense-quality-20261013.log`。用户随后指出重构目的不够突出，本轮补明全局注入→实例登记/目标路由、统一 access 核验/存证、后台及资源归属，重新独立审查、重读 owner/图并复跑字数/用例/文档/质量检查；未通过前不勾选，文档不代替插件实现或部署验收。
- 本轮验证结果：最终可见正文含标题 983 字符（不含空白、Markdown 标记及链接地址）；独立审查通过，明确逐请求选一次实例，平台与源侧权限分别成立，保留服务没有转嫁接入方。owner/图已重读，12 用例 ID/标题原样提取，`make check-docs`、`make quality`（退出 0）及本轮空白检查通过；质量日志 `/tmp/kc-resource-access-plugin-focus-quality-20261013.log`。仅文档及图标题变更，登记/路由、直接 access 补齐与生产保留仍未实现。
