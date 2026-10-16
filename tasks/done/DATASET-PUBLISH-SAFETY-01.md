# DATASET-PUBLISH-SAFETY-01 · Dataset 发布重试幂等与发布前对象冲突拒绝

## 目标

手动发布 Dataset 时两件最常碰到的事有确定结果：响应丢失后重试同一次已接受的发布，得到幂等确认而不是
`NON_FAST_FORWARD`，也不产生新修订；同一对象的同一方面从两个来源仓选入时，发布被拒并指出对象、方面和
来源，不能等到消费者读取时才第一次报冲突。

## 依据

- [Dataset](../../doc/design/dataset.md)：组合规则（同一对象同一方面来自两个来源仓时拒绝发布）、发布、
  进度表“近期：同一次 Dataset 定义的重复提交幂等”。
- [服务](../../doc/design/service.md)：写回执与幂等结果须保持。

## 来源

2026-10-16 从 `DATASET-REVISION-01` 拆出：该任务的“并入”一节收了 `DEBT-DATASET-DEFINE-IDEMPOTENCY-01`
与 `DEBT-DATASET-CONFLICT-ADMISSION-01`，两条都不依赖按 Dataset 修订建检索空间，却被该任务的前置
`INDEX-SEARCH-SPACE-01` 一起挡住。自动跟随会大量重试发布，幂等必须先于跟随成立，所以拆成它的前置。
两条债的位置、信号与判定见 `done/` 中原文。

## 验收

- `cli/deployment_dataset_recovery_test.go`：重试同一已接受发布返回幂等确认，修订不变；配方不同的同修订
  号仍被拒绝，版本单调守卫不放宽。
- 无检索索引时，两仓选入同一 Schema（或同一对象同一方面）的 Dataset 定义被拒，错误指出对象、方面与来源；
  不相交方面组合、单份 Schema 选择与纯文件交付仍被接受；候选失败不替换已接受旧修订；Reader 的冲突检查
  不削弱。
- 受影响的既有用例按新规则改写夹具而不是放宽断言。
- `make check-docs`、`make quality` 与受影响包测试通过。

## 前置

无。

## 不做

不做跟随（`DATASET-REVISION-01`）、检索保留与退役（`DATASET-RETENTION-01`）；不新增自动去重；不改对象身份。

## MVP 收尾核对与实现（2026-10-16）

- 按 Dataset 的发布与组合规则、服务的恢复与幂等结果核对当前 WIP：同修订请求重放已经接入本地和 HTTP
  发布路径；对象方面冲突由 `home.publicationConflicts` 经 `discovery/datasets.Preparation` 在索引开关之外
  检查，候选通过后才接受，不再把旧债记录当成尚未实现的能力。
- 核对发现一处仍会把不同发布确认成成功的缺口：`ReplayedDefinition` 原先无条件抹去请求中的 `Commit`。
  但显式 Commit 会选择固定来源，不能把从已接受 `c1` 改成 `c2` 的同修订请求确认成 `REPLAYED`。
  当前补入显式提交一致性检查；省略 Commit 的分支重试仍复用已接受版本，HEAD 前进不会使其重新冻结。
  请求显式指定同一个已接受提交也可重放，不新增请求身份或公开协议字段。
- 补 `snapshot/dataset/define_replay_test.go` 的同提交重放与不同提交拒绝反例；新增
  `snapshot/service/dataset_replay_http_test.go` 经公开 HTTP 验证 HEAD 前进后的分支重放、同提交重放、
  不同提交得到 `NON_FAST_FORWARD`，且均不触发来源准备、不改登记 HEAD、修订或历史。
- `snapshot/dataset/README.md` 同步说明显式 Commit 与省略 Commit 的边界并重读；设计未改。
- **实现已落地，验证待统一执行**：按用户要求本轮仅修改及格式化，未跑测试或门禁；任务保留 doing。
  统一阶段至少执行下列定向范围，再执行 `make check-docs`、`make quality` 与受影响完整合同：

```bash
go test ./snapshot/dataset -run '^TestReplayedDefinition' -count=1
go test ./snapshot/service -run '^TestDatasetHTTP(StandaloneManagementLifecycle|PublicationReplayKeepsAcceptedCommit|PublicationPreparesThenSwitchesOnce|PreparationFailureKeepsServingVersion)$' -count=1
go test ./cli -run '^TestDataset(PublishingRecoveryKeepsAcceptedVersion|PublicationRejectsSchemaHeldByTwoSources|PublicationComposesDisjointSources)$' -count=1
```

上述计划尚未执行，不用旧定向结果充作本次验收；未提交、未安装依赖或启动真实部署。

## 交付

2026-10-16 按本条自身验收收口：手动发布在当前授权后识别已接受定义的重试，返回 REPLAYED，不新建修订或回退当前版本；不同配方仍由单调版本守卫拒绝。请求显式 Commit 必须等于该修订已接受的固定提交，省略 Commit 的分支重试保留原冻结结果。对象方面和 Schema 整值冲突在不依赖索引的发布准备中拒绝，候选失败保留旧版；不相交方面、单份 Schema 与纯文件交付继续成立。

已验证：b16 受影响回归（`.validation/runs/20261016-mvp-priority-closeout/gates/20261016T045451Z-a5016df50e0b/report.json`） 63 pass / 0 fail / 0 skip，包含发布恢复、两仓 Schema 冲突、不相交来源、重放 helper 与公开 HTTP 的 HEAD 前进、相同显式提交及不同显式提交反例；该来源相关实现和新增回归字节已核对与后续 8667 候选一致。8667 Dataset 全包与 quality 报告（`.validation/runs/20261016-mvp-unified-050129Z/dataset-quality-report.json`） 为 Dataset 210 pass / 0 fail / 0 skip，make quality（含 check-docs）及所列 Python 合同均通过、sourceChanged=false。较早 374 来源合同报告（`.validation/runs/20261016-mvp-unified-043602Z/gates/20261016T044027Z-d94f92fb680a/report.json`） 中未改变的发布准备、方面冲突与权限合同通过；不相交范围和旧版保持断言未削弱。

受影响包的既有发布合同、新增公开 HTTP 边界及当前 Dataset 全包已分别覆盖本条验收，不要求再等待与本条无关的完整场景或外部 live。不同来源的基线合同与增量回归分别记账，不据此宣称 8667 的 Server、CLI 或全仓完整入口全绿。真实 lakeFS 部署不在本条自身完成条件中，本条也未宣称其通过。未提交、未安装依赖；本节更新上文“验证待执行”的阶段结论，目录由统一收口处理。
