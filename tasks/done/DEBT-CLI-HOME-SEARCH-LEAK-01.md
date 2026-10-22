# DEBT-CLI-HOME-SEARCH-LEAK-01 · cli 与 home 夹具整包运行留下无主投影索引

## 债

- 位置：`home/config/stores.go:69` 的 `DefaultStores`——看到 `KC_TEST_OPENSEARCH_URL` 就选
  `opensearch` 驱动并设 1 主分片，但 `OpenSearch.Namespace` 保持空；`internal/testkit/testkit.go:21`
  的 `TempDir` 只删本地目录，不管远端索引；`cli/` 里直接读该变量建引擎的 live 用例十余处
  （`relations_flow_test.go:83`、`traverse_flow_test.go:115`、`search_expression_live_test.go:59`、
  `search_semantic_http_live_test.go:61`、`rerank_http_test.go:223`、`state_search_live_test.go:19`、
  `projection_notify_test.go:57`、`projection_recovery_test.go:29`、`serve_shutdown_fixture_test.go:39`
  等）。
- 信号：在独占自动实例 `kc-local-opensearch-8866dea7622f` 上运行 `go test -short -count=3 ./cli`，
  实例内新增 327 个 `kc-proj-<hex>-g-<hex>` 索引，形状与共享实例上 2026-10-21T12:33Z–12:59Z 一次
  `make test-contracts` 留下的 411 个完全一致。2026-10-22 在共享实例 `127.0.0.1:19200` 实测复现：
  `go test -short -count=1 ./cli` 结束后索引 606 → 811，新增 205 个，全部是两段式 legacy 名
  （无 namespace 派生、1 主分片、0 副本）；同一次会话里 `go test -short -count=1 ./home` 结束后
  606 → 606，新增 0。控制索引里可见来源仓库名形如 `kr://acme/public/core/index-test-<nano>-<seq>`、
  `dataset-index/observation-search/...`、`kr://conformance/es/*`，即 live 夹具而非产品路径。
- 判定：不是测试写坏，是所有权缺口的第二半。空 namespace 按
  `declarativeindex/opensearch` 的 `Config.Namespace` 合同采用 legacy 共享资源，这批索引永不删除；
  `./cli` 整包没有包级清理，每次运行都重新累积。`declarativeindex` 侧的泄漏已由前一阶段修复
  （夹具经 `testfixture.LiveNamespace` 派生 namespace 并在 `t.Cleanup` 里精确删除，实测
  `606 → 606`）。`home` 在 `-short` 下实测不产生索引，但机制上同样依赖空 namespace，属同一缺口。
  `DEBT-SHARED-SEARCH-UNOWNED-GROUPS-01` 落地的 `scripts/opensearch_fixture.py guard` 已在
  `component`/`boundary`/`e2e`/`contracts` 四个组入口按运行前快照回收新增，但直接 `go test ./cli`
  绕过 `make` 入口，所以组边界不能替代包内回收。
- 处置：已落定并实现——**包级差集回收**，与组入口 `guard` 同一规则，不引入测试级命名空间。
  判定过程排除了两个备选：测试级命名空间需要产品侧 `DefaultStores` 派生资源名，等于把测试意图写进
  产品合同；包内统一派生会让两个 home 的同一仓库名撞到同一物理索引，改变测试语义。差集两者都不需要。
- 触发条件：任何一次在 `KC_TEST_OPENSEARCH_URL` 指向长期实例的环境里直接运行 `go test ./cli`、
  `go test ./home`，或走 `make test`（`lakefs` 组只跑两个场景用例，影响面小于 e2e 段）。

## 目标

cli 与 home 的测试夹具在共享实例上运行后，本组拥有的投影索引与分片回到运行前水平，不需要依赖
外层组入口的事后差集回收；做不到时入口明确要求专属实例，而不是默默占用共享分片预算。

## 依据

- [测试与验证](../../doc/engineering/testing.md)
- [工程质量闭环](../../doc/engineering/quality.md)
- 已成立的所有权合同：`DEBT-SHARED-SEARCH-UNOWNED-GROUPS-01`（组入口 guard 差集回收）与
  `DEBT-TEST-OPENSEARCH-LEASE-01`（自动实例租约）

## 验收

- 在 `KC_TEST_OPENSEARCH_URL` 指向已运行实例的情况下直接运行 `go test -short -count=1 ./cli`，
  结束后该实例索引集合与分片计数与运行前一致，且不删除运行前已存在的索引。
- `go test -short -count=1 ./home` 同一条证明；走默认 `DefaultStores` 的用例也覆盖到，不只
  `connected_plugin_fixture_test.go` 那一条自派生 namespace 的路径。
- 离线或本地回归覆盖清理路径，不依赖 Docker 与真实搜索实例。
- 不引入后台 worker 与索引删除的竞争：清理前后不得出现 `tx.Bucket` 为 nil 的写入路径。

## 前置

无。与 `ACCEPT-HARNESS-01` 共用同一共享实例，执行时不能同时占用。

## 不做

- 不删除本轮之前已存在的无主残留；那批索引需要单独的归属裁决，见
  `DEBT-SHARED-SEARCH-UNOWNED-GROUPS-01` 的“不做”条款。
- 不改 `DEBT-SHARED-SEARCH-UNOWNED-GROUPS-01` 已成立的组入口差集回收语义。
- 不改产品侧 `opensearch.Config` 的 namespace 合同，不为测试方便让生产路径派生资源名。
- 不提高 `cluster.max_shards_per_node`，不加 skip，不为让门禁变绿而降低断言。

## 交付（2026-10-22）

处置落定：cli 与 home 各自在 `TestMain` 领取一次运行所有权，`m.Run` 结束后只删本次新增，既存在
的不动。实现三处：

- `internal/testkit/opensearch_process_namespace.go` 新增 `OwnSharedSearchIndices()`：读取端点上的
  `_cat/indices/kc-proj*` 快照，返回的收尾函数求差集后逐个精确 `DELETE`（从不通配）。没有显式端点
  时返回 nil，不假装拥有资源。清单前缀取 `kc-proj*` 而不是 `kc-proj-*`，因为 legacy 控制索引
  `kc-projection-control-v1` 也是按需创建、也必须回收——第一次整包验证正是漏在这一点上：
  205 个投影索引都被删掉，只剩 `kc-projection-control-v1` 存活。
- `cli/testmain_test.go` 与新建的 `home/testmain_test.go` 在 `m.Run` 前后各接一次，回收排在
  `cli.CloseComponentFixturesForTest()` 之后，让所有 home 先关闭再删索引。
- `declarativeindex/control/store.go` 的 `failHead` 补 nil bucket 保护。这不是本次改动的引子，而是
  排查过程中确认的独立缺陷：`finish` 早已有同一保护，`failHead` 在恢复路径上却没有，任何“journal
  被重建而 worker 仍持有句柄”的场景都会直接段错误。回归
  `store_failhead_test.go` 先证伪（去掉保护时 panic 在 `store.go:78`）、再证真。

一条必须记录的实现缺陷（本轮自伤）：初版只按“本进程快照”回收，忽略了 `cli` 的 `serve` 生命周期
用例会把测试二进制重新执行为独立服务进程。子进程的 `TestMain` 于是按自己的快照把父进程仍在断言的
索引删了，`TestServeSIGTERMCancelsMaintenanceAndRecoversOnRestart` 稳定失败在
“restart did not publish a usable complete projection”。定位方式是临时撤销本次两个文件的改动、在
同一实例上复跑该测试——HEAD 通过、改后失败，据此确认是回归而非环境差异。修复是用运行标记
`KC_TEST_OPENSEARCH_OWNER` 表达所有权：父进程写入快照摘要，子进程继承后只读不认领。

验证（全部在自动实例 `scripts/opensearch_auto.py` 上，避免占用共享实例的分片预算）：

- `go test -short -count=1 ./cli` 通过（约 730s），运行前 1 个索引、运行后 4 个，新增 0——
  剩下的是 `.opensearch-observability`、`.plugins-ml-config`、`.opensearch-sap-log-types-config`
  与按日期滚动的 `top_queries-*`，都不是投影索引。
- `go test -short -count=1 ./home` 通过，运行前 2 个、运行后 3 个，同样新增 0。
- 离线回归 `internal/testkit`：只删新增、预先存在的控制索引与投影索引都保留；子进程不重复认领；
  未绑定端点不认领。`go test ./internal/testkit/` 全绿。
- `go build ./...`、`gofmt -l`、`go run ./scripts/check-docs` 均通过。

一处环境事实，留给下一次排障：共享实例 `127.0.0.1:19200` 在本次验证前已达 829 个索引 / 849 个
分片，`cluster.max_shards_per_node` 为 1000，`./cli` 整包需要约 205 个分片，因此在共享实例上
无法完整跑通（实测中途 `USAGE_INVALID: this action would add [1] total shards`）。这不是本次改动
引入的，而是本债本身要防的后果已经发生。同实例的 legacy 索引里混有本地部署在用资源
（`.kc`/`.kc-home` 与 compose 部署都指向该实例，部分控制文档指向真实仓库名），因此没有就地清理，
历史残留的归属裁决仍然独立。
