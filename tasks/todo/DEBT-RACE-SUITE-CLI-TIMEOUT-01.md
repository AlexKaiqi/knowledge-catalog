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
