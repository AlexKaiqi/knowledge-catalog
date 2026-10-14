# handler

LiveAccess 插件内部的处理器机制：固定合同定义、实现声明与处理器注册表。对外没有 Interface 概念；用户只看到"插件"。`liveaccess/handler` 只被 LiveAccess 与其装配点引用，不是第二个插件扩展点。

## 定义与注册

根定义解释固定合同、实现与引用，`registry.go` 保存处理器及匹配合同。Handler 只获得固定范围的 `DeclarationReader`，没有 Store 写口、动态 HEAD 或 Dataset 权威。合同摘要、实际实现 Schema 结构与输入/结果结构在调用时验证；注册不发权，不执行来源 I/O。

`DiscoverObjectInterfaces` 只发现给定对象的实现 Members；合同引用使用同仓对象身份或已在本次 Workspace 明确固定的跨仓引用，不挂载新仓或读取 HEAD。`standardImplementationSchemas` 只推荐实现形式，实际支持由处理器注册摘要及实现检查决定。

`ResolvedInterface.ImplementationSchemaDigest` 保留完整 Schema 对象摘要；`ImplementationSchemaStructureDigest` 排除字段 `access`，用于处理器匹配。两者来自同一次固定读取，查询声明与执行结构分别核验。

## 调用任务

调用由 [dispatch](../dispatch/README.md) 执行。部署不能注入额外处理器：合同不是 LiveAccess 时调用以 `CAPABILITY_UNSATISFIED` 拒绝，说明没有已安装插件处理该合同。
