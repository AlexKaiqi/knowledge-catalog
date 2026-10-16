# DEBT-TEST-OPENSEARCH-TIMING-01 · 自动搜索夹具离线进程超时归因

## 债

- 位置：`scripts/opensearch_auto_test.py` 的 `wait_ready`、`stop_children`，及调用它的
  `internal/arch/opensearch_fixture_lifecycle_test.go`。
- 信号：本地客户端打包后的完整合同中，`test_sigterm_interrupts_a_waiter_without_starting_another_instance`
  首个 fake fixture 8 秒内未到测试子进程，随后清理 `communicate(timeout=3)` 再次超时。
  原失败（`.validation/client-package-contracts.log`）保留完整 fake 事件；同源
  孤立复测（`.validation/client-package-isolated-lifecycle.log`）通过（25.22 秒）。
- 判定：存量离线测试可靠性信号，根因未确定；孤立复测通过不能证明生命周期无故障，也不能据此归因为并行负载。
- 处置：并入已有自动夹具资源合同所在的 `ACCEPT-HARNESS-01`，保留首次失败和复测证据；区分
  fake 命令启动、就绪与子进程清理各阶段的阻塞，再按根因修复实现或等待同步，不用 skip 或删减断言。
- 触发条件：`ACCEPT-HARNESS-01` 已在 doing 且拥有同一自动夹具与生命周期验证入口，登记后立即并入。

## 目标

自动搜索夹具的离线信号合同在完整 boundary 中稳定执行；就绪与清理失败具有可归因证据。

## 依据

- [测试与验证](../../doc/engineering/testing.md)。
- [工程质量闭环](../../doc/engineering/quality.md)。

## 验收

- 定位首次未就绪和清理超时的实际阻塞阶段；正常与受控进程延迟下的断言保持同一语义。
- SIGTERM 等待者不启动第二实例、所有权清理和退出码断言保留；离线专项及完整 boundary 通过，无新增 skip。
- `make check-docs`、`make quality` 通过；不把一次复测通过写成根因已修复。

## 前置

无；与 `ACCEPT-HARNESS-01` 共用验证入口。

## 不做

不借此扩大客户端安装包范围，不启动或操作其它真实搜索实例，不安装依赖。

## 关闭

登记后立即并入 `ACCEPT-HARNESS-01`；信号、证据及剩余验收在主任务继续，本条关闭不表示问题已修复。
