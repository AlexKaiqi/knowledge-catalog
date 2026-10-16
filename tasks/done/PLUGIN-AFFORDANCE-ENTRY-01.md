# PLUGIN-AFFORDANCE-ENTRY-01 · 插件说明入口与目标访问概览（2026-10-13）

- [x] 已将能力说明读取写入插件重构合同并同步 CLI owner：明确 `kc help` 基础能力导航、`kc plugin <名称> help` 说明读取，以及 Dataset 知识访问概览中实际选用插件的用途和帮助入口；更新发现过程、例子、改接表及验收。
- 范围：只改共享插件篇、CLI 设计、相关图主题和本条；不改两个插件自身目标、Go、公开协议或运行配置，不安装依赖、不启动服务、不提交。Object 提供基础模型与 Schema；插件提供自己的 Affordance，应用层按真实选用组装概览，纯文件 Dataset 继续独立。
- 处置：此前解释把需要使用说明直接推成请求时 describe Hook，判定为无必要的执行接点；本轮改为登记可复用说明、按目标交付与按需读取。说明入口和目标聚合尚缺公开合同及接线，重构实施时由 CLI/消费服务落实类型、README/Conformance 和真实走读，不把现有 SchemaReport 或文档绿色当成已实现。
- 验证出口：独立复核职责与端到端 discover/access 过程，重读变更 owner/图，运行 `make check-docs`、`make quality` 和本轮空白检查；全绿后只标记本次文档完成。
- 完成证据：独立审查未发现实质问题，新增说明/概览均标为待实施，固定 Schema 展示、运行实例选用、纯文件 Dataset 与真实执行职责分开。`make check-docs` 通过（33 文档 / 171 关系），`make quality` 退出 0（日志 `/tmp/kc-plugin-affordance-quality-20261013.log`），增量及新增文件空白检查通过。只完成文档，不证明新命令、说明读取服务或目标概览接口已经实现。
