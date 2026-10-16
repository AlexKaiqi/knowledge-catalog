# LAKEFS-02 · lakeFS 写入通路并发化与提交回执恢复

- [x] 本轮完成：`ApplyTreeCommit` 改为 wip 分支上有界并发 stage（默认 32，`KC_LAKEFS_STAGE_CONCURRENCY` 覆盖，首错误中止，commit/publish 仍单次串行）；commit 存在性探测加有界正向记忆（绝不缓存负向）；远端提交 POST 结果未知（超时/连接中断/`ErrTemporaryUnavailable`）时客户端按 `KC_COMMIT_RECEIPT_WAIT`（默认 10m，0 关闭）轮询既有 `GET /writer/v1/receipts/{command}` 恢复权威 receipt，确定性拒绝不轮询。红证据（串行 max in-flight=1、同一 commit 三次 ReadFile 探测三次、POST 超时丢 receipt）全部先行复现后转绿；`snapshot/lakefs` 全包含共享合同与 -race、`cli` 恢复用例与整包、`make check-docs` 通过。`make test` 各执行组（TestMetricPermissionScenes、TestProductScenes、`ok kc/cli`）全部通过，但验证包装器因并行 DATASET-03 工作线在运行窗口内修改源树而按 `source-changed` 判废（记录保留 `.validation/runs/20260922T031949Z-7f3ce8cc969a`）；并行线安静后的指纹稳定重跑不由本条宣称。真实 lakeFS/COS 吞吐与 staging 并发行为归 SCALE-01 部署验证。

**Goal：** 同一契约下把 `ApplyTreeCommit` 的对象写入改为有界并发（staging 原语不变：wip 私有分支上并发 stage/delete，commit 与 publish 仍单次串行），commit 存在性探测按不可变 commit 做正向记忆；远端提交 POST 因超时或连接中断失败时，客户端凭 command_id 轮询既有 `GET /writer/v1/receipts/{command}` 恢复权威结果，不再把「结果未知」当「提交失败」。

**Non-Goals：** 不改 staging/presigned 数据面合同（预签名地址不越过 Server 边界）；不改 Writer ChangeSet 合同、CAS、命令幂等账本语义或公开 CLI/HTTP 形状；不新增 HTTP 路由；不做 ChangeSet 压缩与批量差分端点（后续单独认领）；不宣称生产容量资格（归 SCALE-01）；不为适配并发放宽发布锁语义。

**不变量：** `A-01`、`V-01`、`W-01`、`W-02`；expected-old CAS 与命令幂等保持；同一 commit 读取结果逐字节不变；并发写入遇首个错误仍中止本次 apply 并清理 wip 分支；receipt 恢复只读取权威账本，不重放写入。

**选定与否决：** 选定 wip 分支上有界并发 stage（默认 32，`KC_LAKEFS_STAGE_CONCURRENCY` 覆盖）+ 首错误中止 + 正向 commit 存在记忆（只缓存「存在」，绝不缓存「不存在」；属 `CA-01` 允许的 transport cache）；客户端在结果未知类失败（超时/连接中断/`ErrTemporaryUnavailable`）后按 `KC_COMMIT_RECEIPT_WAIT`（默认 10m，0 关闭）轮询 receipt，确定性拒绝（其余 kernel 错误码）不轮询直接返回。否决客户端直传对象存储（预签名越界）；否决 import 零拷贝路径（字节在客户端且必须过 Writer）；否决把 receipt 轮询改成服务器 202 异步执行（本轮不改 HTTP 形状）；否决负向 commit 缓存（会吞掉并发创建的新 commit）。

**接口与验收：** 公开形状不变（`snapshot.TreeChangeSet` / `writer.CommitReceipt` / CLI argv）。先运行会失败的证据：staging 并发度计数（串行实现 max in-flight=1）、同一 commit 重复 `ReadFile` 的 commit 探测次数（现状每次一次）、POST 超时后客户端丢失 receipt；再实现；以 `snapshot/lakefs`（含 -race）、`cli` 定向回归与 `make check-docs` 验收，不以 skip 换绿。
