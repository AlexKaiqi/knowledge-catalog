# 已配置但未接入的业务源

construct 准备仓与关系仓的既有 Snapshot binding，确认 登记空间 仍只登记 System Repository。两仓前态由未登记边界 probe 与后续 attach 共用。System 信任根已在初始化时提供；Schema 浏览、正文回读与不可写验证挂在初始化前态。

两仓都不携带插件选择。`Given existing repository <id>` 只准备墙外 Snapshot binding，不是新增 CLI 参数；夹具只接受一个仓 id，带上已退役的按仓插件选择后缀会在解析时失败。插件实例由部署配置给出：装了 `liveaccess` 的部署按取值描述的 `runtime` 与 `protocol` 认领实时源，`attach` 只验证并登记来源。实时源无需已在线，后续离线访问用例仍验证来源不可达的原错误。
