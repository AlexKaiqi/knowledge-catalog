# LAKEFS-04 · bulk-ingest：一次性大量小文件的 lakeFS 批量写通路

- [x] 已完成：bulk-ingest 数据面能力已实现并验证，2026-09-28。选定为 lakeFS 对象上传 API（实现前核验否决 import 通路，见「选定与否决」三条理由）；`snapshot.BulkTreeIngester` 可选能力 + `knowledge.ChangeSet.BulkIngest`（`omitempty` 不动既有 digest）+ `writer commit --bulk` 单一显式选项贯通 CLI→Writer→adapter，缺能力确定性失败关闭，通路翻转落既有 IDEMPOTENCY_CONFLICT。已验证：fake 介质合同+bulk 三用例与 `-race`、`object/writer` bulk 三用例、CLI 守卫与 wire 三用例、全量 `make test`（exit 0）、`make check-docs`、`make quality`；真实 lakeFS 部署栈（compose scenes 容器，Graveler+MinIO）目标性验证 bulk PUT/REMOVE/CAS 与回读（`snapshot/lakefs/write_bulk_live_test.go`，缺环境时条件跳过）。未宣称：`make deploy-local-scenes` 全量走查与 staging/bulk 双路径容量数字（归 SCALE-01）。

**背景：** 当前 lakeFS 数据面只有逐对象 presigned staging——每文件 presign、PUT、link 三次往返，32 并发（`KC_LAKEFS_STAGE_CONCURRENCY`）只摊常数不消量级，commit 时长随变更条目数线性；smoke 证据（`.data/scale/runs/smoke-20260922T101955Z`）仅到 100 文件点已 18.9s，10w 文件点从未实测。LAKEFS-02 否决的是「字节在客户端的 import 零拷贝」（绕过 Writer），本条维持该否决：字节仍然只经 Writer 进入介质，变的只是「对象如何进介质」这一个数据面原语。

**Goal：** 为 `snapshot/lakefs` 增加 bulk-ingest 可选数据面能力：Writer 对显式 opt-in（`writer commit --bulk`）的 ChangeSet 以 lakeFS 对象上传 API（`POST/DELETE /branches/{b}/objects`，每文件 1 次往返，字节经 lakeFS 服务端落到 backing store）替代逐对象 presigned staging 三次往返；wip 分支、commit、publish（lock-branch hard-reset）与 receipt 恢复全部复用现有协议。公开 Writer/ChangeSet/CAS/幂等账本语义不变。

**Non-Goals：** 不改默认（无选项）staging 路径与发布锁语义；不给客户端对象存储直传口（维持 LAKEFS-02 否决）；不把 COS 凭证引入 KC Server；不新增公开 HTTP 路由与 CLI 动词（仅 `writer commit` 增单一 opt-in 选项，见设计问题 1）；不做 v1 自动路由与部署级模式旋钮；不宣称生产容量资格（归 SCALE-01）。

**不变量：** `A-01`、`V-01`、`W-01`、`W-02`；字节只经 Writer 进入介质（新增，随实现与测试转正登记）；bulk-ingest 是 adapter 数据面能力，不是协议旁路——「规模权威提供方不得暴露绕过知识不变量的原始路径写入口」（[架构总览](../../doc/architecture.md) 硬性约束）保持成立；bulk apply 失败/中断不留半提交，重放按既有 receipt 幂等语义收敛。

**选定与否决（拟，实现前逐项复核）：** 选定两模式并存、显式选择（无选项=staging 增量小 commit；`--bulk`=一次性大量条目）+ `snapshot.Store` 可选能力 `BulkTreeIngester` + lakeFS 对象上传 API 作为 bulk 数据面。**否决 lakeFS import API 通路**（实现前核验发现三处硬冲突）：① import 源须在 COS 自选键位且键名保持树路径，而 staging presign 的物理地址由介质自选，KC Server 按 Non-Goal 不持 COS 凭证，该通路在不违反凭证合同的前提下无法成立；② import 目的地前缀是替换语义（官方指南 Note：目的前缀下既有对象会被删除），与 expected-old CAS 冲突，补录/重跑会误删既有条目；③ import 不表达 REMOVE。另否决：客户端直传 COS 与 import 零拷贝绕 Writer（维持 LAKEFS-02）；把 bulk-ingest 做成协议级新写入面（不新增 APPEND/PATCH 类 Surface）；以打包档案为介质对象（Graveler 逐条目，介质不解包）；v1 自动按条目数路由与部署级 `auto|staging|ingest` 旋钮（阈值无实测依据、静默切换扩大新通路爆炸半径、单一机制够用不加机器；SCALE-01 数据出来后显式被证明不够用再议自动）。

**要回答的设计问题（实现前定案）：**
1. 策略选择（定案）：显式 opt-in，v1 不做自动路由。`writer commit` 新增单一选项（`--bulk`）；缺 bulk-ingest 能力的 adapter 对该选项确定性报错（能力不足由协议明确拒绝，不由上层猜测或模拟成功），无选项时行为与现状逐字节一致。通路选择随命令进既有命令账本：重放按账本记录的通路收敛，同 command_id 通路不一致按既有幂等冲突族确定性拒绝；不加新公开 receipt 字段。
2. 通路与账本对应（定案）：`--bulk` 落在 `knowledge.ChangeSet.BulkIngest`（`omitempty`，false 不进 canonical JSON，既有 digest 不变），随 ChangeSet 进 CanonicalDigest——同 command_id 翻转通路自动落入既有 IDEMPOTENCY_CONFLICT 族，账本无需新字段；bulk apply 与 commit/publish 全同步，无 importStart 类异步状态。
3. 能力合同形状（定案）：与 `DirectoryReader`/`HistoryStore` 同型的 `snapshot.Store` 可选能力（`BulkTreeIngester` + `BulkTreeIngesterOf`）；adapter 层如实实现或不实现；转发型 wrapper（`connectedSource`、telemetry、testkit）经 call-time 断言转发，缺失时确定性失败关闭，不静默。
4. 部署前置核验（定案）：对象上传 API 是 lakeFS OSS 标准 REST（v1.86 基线内），无需 import 的 admin-only/`blockstore.local.import_enabled` 前置。
5. 残留归属（定案）：无 `kc-ingest` 临时前缀——上传对象由 lakeFS 落位 backing store；失败 wip 分支沿既有 `defer deleteBranch` 清理，未提交残留归介质 GC 管辖（与 S3 gateway 弃写同责，非 KC 新增负债）。

**接口与验收：** 公开形状由 `snapshot.Store` 能力合同拥有（[架构总览](../../doc/architecture.md) snapshot-authority-and-derived-media / provider-capability-contract 主题；`snapshot/lakefs/README.md` 数据面节随实现补两模式说明）。先运行会失败的证据：缺 bulk-ingest 能力的 adapter 收到 `--bulk` 时确定性拒绝、bulk 通路未走对象上传 API 或复用 staging 往返、bulk 与 staging 产出内容不一致、同 command_id 重放通路不一致被拒；再实现；以 `snapshot/lakefs` 合同与 -race、幂等重放/恢复用例、`make check-docs` 验收，不以 skip 换绿。

**依据：** [架构总览](../../doc/architecture.md)（写边界与能力合同 owner）、[snapshot/lakefs README](../../snapshot/README.md)（数据面合同）、[规模门槛](../../doc/engineering/testing.md) 与 SCALE-01（探针与双路径对照）、lakeFS 官方 [Import 指南](https://docs.lakefs.io/guides/import/) 与 [性能最佳实践](https://docs.lakefs.io/concepts/performance-best-practices/)（import 推荐口径与其目的地替换语义 Note，2026-09-24 读）。
