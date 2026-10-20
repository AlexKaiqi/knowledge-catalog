# DEBT-RACE-SUITE-CLI-TIMEOUT-01 · `make test-race` 在 `kc/cli` 超过 30 分钟超时

## 债

- 位置：`scripts/testsuite.sh` 的 `run_race`（`-race -timeout=30m`，包含 `./cli`）；`cli` 包测试体量。
- 信号：2026-10-19 冻结副本门禁 `make test-race` 退出 2：`kc/cli` 跑满 1800 秒触发 `panic: test timed out`，
  超时时正在跑的是 `TestSceneViews/projection_contract`（才 8 秒），并非卡死在单个用例；同轮其余 10 个 race 包通过。
  同一源码下 `kc/cli` 非 race 运行 641 秒（`make test-contracts`），race 开销约 5～10 倍，必然超过 30 分钟。
  2026-10-19 01:53 的上一次 race 运行也以失败结束。日志：`.validation/remaining-20261019/test-race.log`。
- 判定：race 门禁形同虚设，`cli` 下的并发问题检不出来；不是某个具体数据竞争。
- 处置：把 race 范围收到 `cli` 中真正并发敏感的用例（按 `-run` 选择或拆出专门的 race 测试入口），或给 `cli`
  单独的超时预算并降低其产品场景在 race 下的规模；不靠删减断言或 skip 变绿。
- 触发条件：下一次修改 `scripts/testsuite.sh` 的 race 组或 `cli` 并发路径时；或需要宣称 race 门禁通过时。

## 目标

`make test-race` 在本机预算内跑完并全绿，覆盖范围有明确清单。

## 依据

- [工程质量闭环](../../doc/engineering/quality.md)：质量问题三个出口。

## 验收

- `make test-race` 退出 0，无 FAIL、无超时；`cli` 的并发敏感用例仍在 race 下执行。
- `make check-docs`、`make quality` 通过。

## 前置

无。

## 不做

不删除或削弱 `cli` 用例断言，不用 skip 换绿。

## 交付

- 处置：`kc/cli` 不再整包进 race。`scripts/testsuite.sh` 的 `run_race` 对 `./cli` 只按
  `scripts/race-cli-tests.txt` 所列用例 `-run`，预算 15 分钟；其余 race 包预算不变，并加入 `./server`、
  `./invocation`，直接对服务端并发路径（SERVICE-CONCURRENCY-01 去掉进程级互斥后的调用与 allow 更新）做 race，
  替代此前经 `cli` 集成用例间接覆盖的部分。`cli` 的场景与产品旅程照旧在 `make test` / `make test-contracts`
  中非 race 执行。
- 升级为自动检查：`cli/race_scope_internal_test.go` 的 `TestRaceScopeCoversCLIConcurrencyTests` 用 AST 扫描
  `cli/*_test.go`，测试体内有 `go` 语句、`t.Parallel()`、`sync.WaitGroup`/`errgroup`，或名字含 `Concurrent`
  的顶层用例必须在清单内；清单中不存在的名字同样失败。`TestRaceScopeScannerRecognisesConcurrencySignals`
  钉住扫描规则。清单另含走 `cli/datasetfs*.go`、`cli/desired.go` 生产 goroutine 的用例。
- 红：从清单删去 `TestSceneResultConcurrentWriters` 并加入不存在的 `TestGoneAway`，守卫报出两者后失败；恢复后通过。
- 绿（2026-10-19，主工作区，源码在运行期间未变）：`make test-race` 退出 0，13 个包全部 ok，`kc/cli` 18.7 秒、
  全组约 2 分钟，无 `DATA RACE`；SKIP 3 个均为环境门控（`TestLiveApplyThroughput`、
  `TestLiveBulkApplyAgainstRealLakeFS`、`TestLiveTaihuAuthentication`）。`make check-docs`、`make quality`
  （耦合棘轮持平）、`make check-validation` 通过。
- 文档：`doc/engineering/testing.md`、`doc/engineering/test-catalog.md` 写明 `cli` 的 race 范围由清单与守卫决定。
