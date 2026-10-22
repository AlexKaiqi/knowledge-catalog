# DEBT-CLI-GLOBAL-TRANSPORT-01 · cli 帮助用例共享全局 transport 的偶发失败

## 债

- 位置：`cli/snapshot_affordance_internal_test.go:24-27` 与 `cli/snapshot_topics_internal_test.go:123-126`
  都保存并替换包级 `http.DefaultTransport`，而 `cli/object_reference_journey_test.go:151` 直接使用它。
- 信号：2026-10-21 的契约 e2e 运行中
  `TestSnapshotHelpPreservesModuleContentThroughPublicEntry` 报
  `public help made 9 HTTP requests`；同一用例在前后两次完整契约运行均通过，单独按名字重复
  `-count=3` 也通过。失败点在计数断言，不在功能断言。
- 判定：`cli` 包内这些用例未声明并行，但契约 e2e 阶段由 `go test ./cli ./snapshot/dataset` 一次运行整包；
  用例体不并发时仍可能在同包的其它用例遗留替换窗口内命中，计数与实际请求归属都不受本用例控制。
  因此这是测试隔离缺陷，不是被覆盖功能的行为回归。
- 复核：按契约 e2e 原参数 `go test -short -count=1 -timeout=60m ./cli ./snapshot/dataset` 复跑时该用例
  未出现在失败列表（其余 19 项为未传 OpenSearch 端点导致的环境失败）；按名字 `-count=3` 也通过。
  失败只在完整套件并行运行时出现，符合隔离缺陷而非稳定回归的判断，故本任务不改该项断言。
- 处置：本任务（`ARCH-DATASET-PIN-01`）只做登记，不改该用例断言，也不放宽计数上限。修复方向是把
  替换范围收敛到用例自己构造的客户端（或把替换与断言放进同一串行区并显式声明不并行），
  不通过删除计数断言来变绿。
- 触发条件：下一次改动 `cli` 帮助/传输相关测试，或再次出现同一条计数失败时并入执行。

## 处置记录（2026-10-21）

`ARCH-HOME-CONFIG-01` 的契约 e2e 再次出现同一条计数失败（`public help made 5 HTTP requests`），
命中本债的触发条件，随该任务一并修复：

- `snapshotHelpRejectingRoundTripper` 增加 `host` 与 `fallback` 两个字段，只计数发往本用例
  自己声明的哨兵主机（`snapshot-help.invalid` / `snapshot-topics.invalid`）的请求，
  其余请求转交 `fallback`。这样断言只覆盖本用例可能发起的行为，
  不再依赖同包其它用例在其替换窗口内的调度。
- 两处替换点同步传入 `fallback: http.DefaultTransport`，保留「帮助不得发起 HTTP」与
  「不创建会话/Home/输出文件」两个正反断言，未放宽任何上限。
- 按名字定向复跑相关用例通过。

残留：`cli` 包仍在用例内替换包级 `http.DefaultTransport`。本债的目标是把替换范围收敛到
用例自己构造的客户端；本次只让计数归属正确，替换范围不变，因此该目标仍未达成，
本任务保持在 `todo/`。

## 目标

`cli` 的帮助类用例不再改动包级 `http.DefaultTransport`，其“帮助不得发起 HTTP 请求”的断言只统计
本用例自己创建的请求；反复运行与整包运行结果一致。

## 依据

- [工程质量闭环](../../doc/engineering/quality.md)。

## 验收

- 反复整包运行 `go test -short -count=3 ./cli` 不出现该计数失败。
- 「帮助不发起 HTTP、不创建会话/Home/输出文件」的正反断言保留；不以放宽上限或删除断言实现稳定。
- `make test`、`make test-contracts` 通过。

## 前置

无。

## 不做

不改帮助输出内容、HTTP 请求路径或 e2e 覆盖清单。
## 交付（2026-10-21）

`cli/snapshot_affordance_internal_test.go` 与 `cli/snapshot_topics_internal_test.go` 不再保存、替换或恢复
包级 `http.DefaultTransport`：

- 两个用例各自用 `httptest.NewServer` 起一个只做计数并一律返回 500 的 `countingHandler`，把
  `KC_SERVER_URL` 指向它。帮助路径若真的发请求必然到达该服务器，`count` 只统计本用例自己的请求，
  与同包其它用例的调度无关。
- 原 `snapshotHelpRejectingRoundTripper`（含 `host`/`fallback`/`calls`）整体删除，包级 transport 的
  替换窗口随之消失，本债目标达成。
- 计数断言与「不创建会话/Home/输出文件」的目录断言均保留，未放宽上限、未删除断言。

顺带核清一处更严重的事实：`prepareCLI` 在 `help`/`--help`/`-h` 分支直接 `stop(...)` 返回，根本不进入
`runRemoteCLI`，所以原来的 transport 替换对帮助路径是死代码——断言实际依赖的是「帮助提前返回」，
而不是 transport 拦截。改用真实服务器后，断言的可观察性来自请求是否到达，不再依赖对实现的偶然假设。

按本债验收条件跑整包三连跑时，又发现并修掉一条同类的测试隔离缺陷（不是本债引入，但正属于本债要消除
的“重复运行结果不一致”）：`cli/remote_dispatch_internal_test.go` 的 `remoteDispatchRoutes` 是包级
`var`，子用例把它直接当作可写 flags 传给 `bindObjectOperands`，后者把绑定到的操作数写回该 map。
第一次重复运行之后包级 map 里就常驻 `reference`，第二次运行 `read` 与 `writer put` 立刻报
`USAGE_INVALID: kc read rejects --reference`。复现：`go test -short -count=3 -run
'^TestRemoteDispatchRoutesSupportedOperations$' ./cli` 稳定失败两次、`-count=1` 通过。修复为每个子用例
复制一份 flags（用例表本身仍是只读声明），修改后同命令 `-count=3` 通过。

已验证范围：

- `gofmt -l cli/` 无输出。
- `go test -short -count=5 -run 'TestSnapshotHelp|TestSnapshotTopic' ./cli` 通过。
- 验收条件本身：在自动租约的独占实例（`scripts/opensearch_auto.py`，租约退出后实例自动清理）上跑
  `go test -short -count=3 ./cli`，结果 `ok kc/cli 2119.990s`、退出 0。用独占实例而不是共享部署实例，
  是因为整包会创建约 330 个投影索引，共享实例的存量占用会让它撞上分片预算——那属于环境失败，
  不能记成用例失败或通过。
- 共享部署实例运行前后容量均为 `{"openShards":426,"maximumShards":1000,"freeShards":574}`。
