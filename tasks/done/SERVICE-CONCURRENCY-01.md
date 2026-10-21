# SERVICE-CONCURRENCY-01 · 服务调用去掉进程级互斥，只在有并发语义处串行

## 目标

Server 不再用一把进程级读写锁串行所有写调用。每个请求的过程账、身份印记与 Writer/TreeWriter/Reader/
Dataset 登记表调用上下文按请求隔离，不再改写进程共享的 Home；互斥只留在语义上确实需要的位置：

- 版本化写入：沿用 Snapshot 条件提交（基点已变即 `NON_FAST_FORWARD`）、lakeFS 发布锁分支与命令账本
  按 command ID 串行，不另加进程锁。
- Dataset 登记表：沿用登记表自身锁与 expected-head 条件保存。
- 本机读改写文件（`allow.json`、`hooks.json`、身份绑定）：按文件路径串行一次读改写，避免丢更新。
- 仓库拓扑（创建、连接同一个部署根下的仓）：沿用 `home/` 既有的按部署根串行的托管操作锁，不新增锁。
- 停机：关闭共享适配器前等待在途请求结束（只是生命周期屏障，请求之间不互斥）。

做完后，不同仓、同仓不同基点的写可以同时进行，读与就绪探针不会被长写阻塞；同仓同基点并发写由
条件提交裁决，恰好一方成功。

## 依据

- [Snapshot](../../doc/design/snapshot.md)：并发提交互不覆盖，基础版本已变就拒绝，由提交方重读后再提交。
- [对象](../../doc/design/object.md)：U17 并发、失败与重试不损坏写入。
- [服务](../../doc/design/service.md)：进度表中“服务调用并发锁按服务模式判断”一项，由本任务改为按请求隔离、
  只在有并发语义处串行。
- [工程质量闭环](../../doc/engineering/quality.md)。

## 验收

- 两个写请求同时在途时互不等待；关闭共享 Home 等待在途请求结束后才关闭。
- 并发请求各自的过程账与 Writer/TreeWriter/Dataset 印记互不串号，共享 Home 上的印记不被请求改写。
- 请求级 Dataset 视图上的发布对共享登记表立即可见，只读视图仍不能持久化。
- 并发更新 `allow.json` 与 `hooks.json` 不丢规则或绑定。
- W-02（`TestT2CommitCAS` 等）与命令幂等相关回归保持绿；`go test -race` 覆盖受影响包。
- 受影响完整回归、`make check-docs`、`make quality` 全绿。

## 前置

无。与 `WRITER-INITIAL-UPLOAD-01` 同涉 `object/writer/`，本任务只新增请求级克隆文件，不改其在途文件。

## 不做

不做多实例部署或跨进程分布式锁；不改条件提交与命令账本语义；不改公开 HTTP 形状与错误码；
不处理大仓上传的线性往返（见 `DEBT-WRITER-UPLOAD-SCALE-01`）。

## 进展

实现范围（2026-10-16）：

- 去掉 `server/service_invoke.go` 的 `lockTypedInvocation` 与 facade 读写锁；换成停机准入（`admitRequest`：
  `homeMu` 下检查 closed 后计入在途数，`closeReadHome` 先停准入再等在途归零）。就绪探针、插件说明与
  能力入口、Snapshot 租约同样走准入，请求之间互不等待。
- 请求视图：`Home.RequestView`、`Writer.ForRequest`、`RawWriter.ForRequest`、`dataset.Service.RequestView`
  共享权威、命令账本、登记表已接受状态与插件，只把 journal 与身份印记私有化；借用视图的
  `Close`/`StopMaintenance` 为空操作，不能装配插件。`invocation.ObserveRequest` 在共享 Home 上盖印到
  请求视图，dispatch 对传入的共享 Home 用它、对本次自开的 Home 仍用 `ObserveHome`；Snapshot 写租约改用
  `RequestView`，不再 `SetJournal` 共享 Home。
- 本机读改写文件：`internal/jsonfile.Locked` 按绝对路径串行一次读改写；`invocation.UpdateAllow`
  （grant、revoke、share、托管仓授权、Snapshot grant 存储）、`hook.Update`（hook add/rm）与身份绑定共用它。
- 文档：`doc/design/service.md` 进度行、`home/README.md`、`snapshot/dataset/README.md`、`internal/jsonfile/README.md`。

按设计裁决改写的旧断言：`server/service_concurrency_internal_test.go` 里的三个用例
（读共享读锁、夹具读互斥、写排斥读）断言的正是本任务要去掉的进程锁，无法改写成等价断言；
已替换为“在途请求互不等待”和“关闭等待在途请求且不再准入”。分类用例保留。
`server/dataset_root_recipe_test.go` 的故障注入 writer 通过嵌入装饰 `RawWriter`，现补上 `ForRequest`
让装饰带进请求视图，断言未改。

已验证范围：

- `go test -race -count=1`：`internal/jsonfile`、`hook`、`identity`、`snapshot`、`snapshot/dataset`、
  `snapshot/service`、`object/writer`、`invocation` 通过；`home`、`server` 在
  `scripts/opensearch_auto.py` 提供的临时 OpenSearch 下通过。
- W-02：`-run TestT2CommitCAS` 等 CAS 回归在 `snapshot`、`object/writer` 通过。
- 新增回归：`TestLockedSerializesReadModifyWriteOnOneFile`、`TestConcurrentUpdatesKeepEveryBinding`、
  `TestConcurrentAllowUpdatesKeepEveryRule`、`TestRefusedAllowUpdateWritesNothing`、
  `TestCatalogRequestViewSharesStateButKeepsItsStamp`、`TestRawWriterForRequestKeepsObserversApart`、
  `TestRequestViewOwnsStampsAndBorrowsLifecycle`、`TestTypedInvocationsDoNotWaitForEachOther`、
  `TestCloseWaitsForInFlightRequests`。
- `make check-docs` 通过。受影响包的闸层检查（vet、staticcheck v0.6.1、gocyclo≤50、文件≤700 行、dupl 0 克隆、
  耦合棘轮 held，propagation 14.31% 与基线持平）通过。

全量门禁（2026-10-19，源码稳定后重跑）：

- `make check-docs` PASS；`make quality` 退出 0（gofmt/tidy/vet/staticcheck/圈复杂度/体积/克隆全过，耦合棘轮 held，
  propagation 14.31% 与基线持平）；`make test`（lakeFS 产品场景，`./cli` 与 `./snapshot/dataset`）退出 0，无 FAIL、无 SKIP。
- 全仓 `go test -short ./...`（临时 OpenSearch 下）：69 个包 ok，只有 `object/service` 的
  `TestExportedSurfaceMatchesAnchor` 失败，原因是另一条在途线新增的未跟踪文件 `object/service/shape_read.go`
  还没同步导出面锚点；本任务的 diff 没有碰 `object/service`。
- `scripts/testsuite.sh race` 没有启动：Docker VM 剩余内存 0.81 GB，低于临时 OpenSearch 所需的 1.34 GB，
  内存被其他线正在跑的 `kc-bench-*` 容器占用，没有任何测试被执行。本任务受影响包的 -race 证据见上文，
  之后源码未变。

## 返工：写仍会挡读（2026-10-19）

验收后复核发现两处请求路径仍互相等待，任务移回 `doing/`：

- Dataset 登记状态：每个请求入口的 `Home.ReadView` 都拿 `registryState.mu` 读锁复制状态，而
  `PublishDataset`/`RegisterRepository`/`UnregisterRepository`/`RetireDataset` 持同一把写锁跨过
  `freezeSources`、文件校验与 lakeFS 提交；排队写者还会挡住后到的读者。registry 自身的 `mu` 也包住
  `Log` 历史遍历和 `CheckAuthority`（就绪探针）的远端读。
- 语义投影缓存：`SemanticProjectionFor` 用进程全局锁包住整个 `BuildSemanticProjection`，一次冷构建
  挡住所有请求的语义读，与写无关但同属“请求互等”。

处置：

- `snapshot/dataset`：已接受的登记版本改为不可变的 `acceptedState`，经 `atomic.Pointer` 发布；读与读视图
  直接取指针，无锁、无拷贝，读到的是取指针时的版本。写在锁外完成冻结与校验，`commit` 只串行内存重检
  （修订号前进、来源已登记且已冻结）和 `saveExpected` 的 head CAS，成功后换指针。registry head 改为原子值，
  `Log` 按调用方版本的 head 无锁遍历不可变 commit；`CheckAuthority` 无锁比较，只有不一致时才在提交锁后复核，
  避免把本进程正在提交的窗口误判为“权威已前进”。
- `invocation`：投影缓存收成 `semanticProjectionCache`，锁只护 map，同 key 单飞、跨 key 互不等待，失败不缓存。

回归测试：

- `TestCatalogReadsDoNotWaitForAHeldPublication`：发布卡在权威提交时，共享服务、读视图、请求视图的
  `Set`/`DatasetVersion`/`HasRepository`/`DumpState`/`Log`、`registry.Head`/`CheckAuthority`、另一条
  `PrepareDataset` 都在 5 秒内完成并读到旧版本；放行后新版本可见，先前的读视图仍是旧版本。
- `TestCatalogConcurrentPublicationsOfOneRevisionAcceptOne`：两条按旧版本校验过的同修订号发布，恰好一条接受、
  一条 `NON_FAST_FORWARD`。
- `TestSemanticProjectionBuildDoesNotBlockOtherKeys`、`TestSemanticProjectionFailedBuildIsRetried`。
- 测试夹具 `catalogMem` 增加 `beforeApply` 钩子，在夹具自身锁外停住提交。

已验证：

- `-race` 全绿：`./snapshot/...`、`./invocation`、`./identity/...`、`./hook/...`、`./object/writer/...`（13 个包）；
  `discovery/...` 与 `declarativeindex/{capability,cli,client,plugin}` 不连 OpenSearch 也全绿。
- `make check-docs` PASS；`make quality` 中 gofmt/tidy/vet/staticcheck/圈复杂度/体积/克隆通过。

未验证 / 阻塞：

- `make quality` 耦合棘轮失败：propagation 14.31% → 14.78%，`snapshot/internal` 不稳定度 0.67 → 0.75。
  来源是另一条在途线 10:22 给 `snapshot/internal/lakefsimpl/client.go`、`client/client.go` 新增的
  `kc/internal/httpjson` 依赖；本返工 diff 只引入标准库 `sync/atomic`、`slices`，不改变包依赖图。
- `home`、`server`、`declarativeindex/{control,projection,opensearch}` 需要真实 OpenSearch，本轮失败的用例
  报错全部是“require the existing real OpenSearch endpoint”。共享 `kc-deploy-local-opensearch-1` 正被
  其他线的 `kc-bench-*` 压测占用，临时 OpenSearch 内存不足，按“共享 OpenSearch 同时只给一条线”未借用。
  这两处放开后重跑 `make quality`、`make test` 与这些包的 `-race`，全绿再移到 `done/`。

自检：无新增特例绕过既有抽象；旧断言未改；`Registry` 私有方法 `history` 增加 head 参数，属包内私有形状。
`home` 托管操作锁（按 Home 目录串行仓库创建/连接）只挡写不挡读，本轮不动。

## 收口（2026-10-19）

两处阻塞已解除：

- 耦合棘轮：`make quality` 退出 0，propagation 14.31% 与基线持平（ratchet held）。
- OpenSearch：`scripts/opensearch_auto.py` 临时 OpenSearch 下，`go test -race -short -count=1` 覆盖 `home`、`server`、
  `declarativeindex/{control,projection,opensearch,opensearch/internal/opensearchimpl}`，6 个包全部 ok，无 SKIP。

全量门禁：冻结副本上 `make check-docs` PASS、`make quality` 0、`make test` 0（无 FAIL/SKIP）、`make test-contracts` 0
（14 个 SKIP 都是缺环境变量的 live/规模用例）。主工作区最终源码上 `make check-docs`、`make quality`、`make test` 全绿，
受影响包 `go test -short` 21 个包 ok、无 SKIP。`make test-race` 整组因 `kc/cli` 在 race 下超过 30 分钟超时而失败
（不是数据竞争，本任务未改 `cli`），登记为 `tasks/todo/DEBT-RACE-SUITE-CLI-TIMEOUT-01.md`。
日志：`.validation/remaining-20261019/`。未提交。
