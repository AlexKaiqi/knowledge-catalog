# CACHE-01 · 同版本正文缓存与独立后台预热

- [x] 已完成：服务内同版本正文缓存、可注册的独立后台消费者、有界热点与可选冷启动预热；2026-09-08。38 项本任务合同均实际执行通过、零跳过，缓存及接缝定向 race 通过。最终 `make test` 零失败，真实 adapter 套件零失败、零跳过；短套件的既有条件跳过不计为通过。实现、错误语义与范围边界见本条及缓存包合同；当时详细报告仅留本机、未入库，不作为当前源码验收凭据。

**Goal：** 在固定 basis 的 SEARCH / RELATIONS / 精确 READ 回读处复用完整 Snapshot 正文，批量回源仅处理未命中项；预热独立于索引，并能在发布、丢通知和进程重启后通过 HEAD 对账恢复。owner 为 `doc/design/service.md`、`doc/architecture.md`、`doc/design/discovery.md`。

**Non-Goals：** 不让 Snapshot/Reader 持有语义缓存实现，不修改 Writer/Catalog 协议、不改变公开 SEARCH 候选与正文语义、不缓存动态 State observation、不新增消息中间件或分布式缓存；不把有界预热解释成全仓覆盖或生产容量资格（`doc/architecture.md`、`doc/design/discovery.md`、`doc/design/liveaccess.md`、`SCALE_ARCHITECTURE.md`）。

**不变量：** 保持 V-01、C-01、CA-01/CA-02/CA-03、PC-01、P-01、R-01/R-02、AUTH-01/AUTH-03、KS-02；旧 pin 不读新版本，命中按当前权限交付，缓存副本不受 selector/State hydrate/交付修改污染，消费请求不构建投影。

**选定与否决：** 选定仓 + commit + 对象/完整 Address 的有界进程缓存、批量 miss 回源、返回副本、后台热点与有限冷启动预热；派生消费者分别跟踪进度与故障，用来源版本变化和对账驱动。否决 ID-only/latest 回退、用索引文档更新替代正文变化、让缓存实现 Retriever、Writer 同步预热以及按仓共用所有派生状态。

**接口与验收：** 窄读取端口由 `knowledge.Hydrator` 拥有，上层实现位于 `object/cache`，查询执行器与 Reader/Serving 仅依赖端口；后台扩展由 `index.SnapshotConsumer` / `Controller.RegisterConsumer` 拥有。先运行会失败的缓存/接缝/独立消费者证据，再实现；以版本/权限/副本隔离、批量回源次数、预热恢复及正式 `make check-docs` / `make test` 验收，不以 skip 换绿。
