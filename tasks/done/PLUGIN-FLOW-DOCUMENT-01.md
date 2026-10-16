# PLUGIN-FLOW-DOCUMENT-01 · 插件流程与接入点重写（2026-10-13）

- [x] 文档重写完成：按用户纠正沿启动、任务调用、变化通知、后台对账和退出解释扩展；每个切入点明确调用者、payload、结果采用及失败后果，补完整过程和扩展步骤。
- Goal / owner：通用接入过程归 `plugin-extension.md`；索引逻辑、维护、查询分别沿 `declarative-index.md`、`index-control.md`、`retrieval.md`，LiveAccess 沿 `resource-access.md`；已按 docs/graph 的 ownerTopics 和 Relation 端点核对直接关系，并核验 Cordis 官方机制。
- Non-Goals：只改文档和主题登记，不改 Go、公开协议、CLI、测试、配置或其它任务在途实现；不安装依赖、不启动服务、不提交。保留现有实例选择、固定依据、资源归属与恢复决定。
- 不变量 / 接口：W-01/W-02、V-01/KS-01、C-01/R-01/R-02、AUTH-01～03/KS-02、P-01/PC-01、L-01/API-01；payload 解释其用途并链接现有公开类型，不在设计 Markdown 编造消息字段或状态机。
- 选定 / 否决：用流程、输入输出、切入点和完整例子组织文档，区分同步服务、变化通知、生命周期；借鉴 Cordis 的上下文与贡献归属。否决未来文件清单作为阅读起点、把所有调用改称事件、以源码链接替代过程说明，以及把设计接口宣称为可用插件 API。
- 验证与质量出口：独立按流程复核，重读变更 owner 和文档图，运行 `make check-docs`、`make quality` 及增量空白检查；观察到的实现差距保留在文档落地表，首个接入触发时补齐并验证，不把本轮文档检查写成插件运行或部署验收。
- 交付证据：全文与直接关系已重读，独立复核后修正现有类型锚点、冻结候选输入及显式 notice 的同步结果合同；`make check-docs` 通过（33 文档 / 168 关系），`make quality` 退出 0（日志 `/tmp/kc-plugin-flow-quality-20261013.log`），增量及新增文件空白检查通过。仅文档、阅读入口和图主题登记；未改 Go、运行测试、部署或提交，通用登记与多实例路由仍待实施。
