# OBJECT-PLUGIN-DOCUMENTS-01 · 四项设计合同成文（2026-10-13）

- [x] 文档交付完成：四项契约、承诺与具体接入已落实到现有四篇主设计文档，设计书 README 提供按四问题阅读的入口；仅文档，未实施登记或路由代码。
- Goal / owner：基础对象合同与迁移归 object-refactor-guideline；通用登记、能力分区和目标选择归 plugin-extension；索引逻辑/维护/请求分别归 declarative-index、index-control、retrieval；LiveAccess 的具体登记及访问/观察合同归 resource-access。所有权与直接关系只在 docs/graph 登记。
- Non-Goals：仅文档，不改 Go/API/CLI/测试/配置/系统出版物，不开发统一宿主或安装管理，不安装依赖、不启动服务、不提交；保留各并行任务在途内容。
- 不变量 / 接口：保持 I-01/V-01、W-01/W-02、API-01、AUTH-01～03、R-01/R-02、PC-01及现有观察保证；公开形状引用 object、discovery、liveaccess 和兼容操作合同，不在 Markdown 编造 DTO、错误码或状态机。
- 选定：四项各归一个主阅读文档，公共规则按能力分区，Repo/Dataset 可同时选择索引与 LiveAccess；类型化目录和登记上下文由各消费侧拥有，实例与资源生命周期明确。LiveAccess 接入不再只留未来方向；动态索引与 LiveAccess 组合必须满足已有来源/观察共享合同。
- 否决：单个目标只能选一个万能插件、按生命周期阶段拆出互不关联实例、统一 role+any 注册表、所选实例缺能力后暗回默认、重复正文制造第二合同、用已有算法或测试夹具宣称接入/生产保留已实现。
- 验证出口：重读改动 owner 与文档图，独立机制复核，make check-docs / make quality / git diff --check 全绿再标记本条完成；不把文档闸记为运行/部署验收。


- 交付结果：Object 篇补描述组织及固定写读链，含提交后结果不确定的原命令恢复；插件篇补索引/LiveAccess 带类型能力分区及独立默认/精确目标选择；索引篇补要求→承诺和阶段贡献；资源访问篇补 LiveAccess 登记上下文/私有目录、read/invoke 与兼容声明链，源地址不被实例覆盖。动态共享索引与所选 Live 来源必须相容，不暗合并；三个具体 Live 接入用例、公开 API 锚点及实现差距同时登记。
- 验证证据：owner、阅读入口及 docs/graph 已重读，独立跨篇复核通过；make check-docs（33 文档 / 168 关系）、git diff --check 与 make quality 全绿，日志 /tmp/kc-four-design-documents-quality.log。未改 Go/测试/CLI/运行配置或系统出版物，不将门禁通过记为插件运行、生产观察保留或部署验收；现有质量债及触发出口继续保留。
