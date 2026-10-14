# Interfaces

普通 Interface 是对象按共同处理合同实现的一套定义。根包解释固定定义、实现与引用，`registry.go` 保存处理器及匹配合同；实际调用、授权及输入/结果验证由 `interfaces/service` 执行，消费者通过类型化调用取得被声明的能力。

Handler 只获得固定范围的 `DeclarationReader`，没有 Store 写口、动态 HEAD 或 Dataset 权威。合同摘要、实际实现 Schema 结构与输入/结果结构在分派时验证；注册不发权，不执行来源 I/O。LiveAccess 是独立实现，本包不导入它。

`DiscoverObjectInterfaces` 只发现给定对象的实现 Members；合同引用使用同仓对象身份或已在本次 Workspace 明确固定的跨仓引用，不挂载新仓或读取 HEAD。`standardImplementationSchemas` 只推荐实现形式，实际支持由处理器注册摘要及实现检查决定。

`ResolvedInterface.ImplementationSchemaDigest` 保留完整 Schema 对象摘要；`ImplementationSchemaStructureDigest` 排除字段 `access`，用于处理器匹配。两者来自同一次固定读取，查询声明与执行结构分别核验。
