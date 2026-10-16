# DEBT-TEST-OPENSEARCH-LEASE-01 · 自动搜索夹具跨进程资源协调

## 债

- 位置：`scripts/testsuite.sh`、`scripts/e2e-opensearch.sh` 与 `scripts/e2e-state-runtime-docker.sh` 的自动临时 OpenSearch 分支。
- 信号：两个独立合同入口各自动创建 512MiB heap 实例，单实例实测约 900MiB 以上；2026-10-16 04:08:37 新实例启动后 VM 可用内存从 814,800,896 B 降到 159,629,312 B，本轮独占门禁被 192MiB 护栏中断。另有共享搜索节点 OOM，不能将全局并行占用归为 lakeFS 缓存。
- 判定：入口间缺少同 Docker daemon 的互斥租约与真实 VM 启动预算；单次运行后的护栏不能阻止并发启动。
- 处置：标准库跨进程租约覆盖创建、测试和清理；拿到租约后读取 Docker VM `/proc/meminfo`，不足或无法确定时明确失败。显式 endpoint 不改行为，不停其它容器、不改变普通测试包并发。
- 触发条件：`ACCEPT-HARNESS-01` 当前环境修复与 `scripts/testsuite.sh` 资源合同补齐；登记后立即并入。

## 目标

所有自动创建临时搜索实例的测试入口按同一 Docker daemon 串行使用资源；失败、中断与清理结束释放租约，原测试非零退出码保留。

## 依据

- [测试与验证](../../doc/engineering/testing.md)。
- [工程质量闭环](../../doc/engineering/quality.md)。

## 验收

- 两个独立进程、不同源码副本不会重叠创建实例；租约延续到清理完成。
- SIGTERM 终止子进程并清理自己实例，等待者随后继续；低预算或缺失 MemAvailable 不创建 Java 实例、不执行测试且非零退出。
- 测试失败码与清理失败均保留明确结果；显式 endpoint 不查询 Docker、不拿自动租约。
- 自动 adapter 不再嵌套创建第二实例；离线 fake Docker 合同、`make check-docs`、`make quality` 通过。

## 前置

无；与 `ACCEPT-HARNESS-01` 共用脚本与测试环境。

## 不做

不启动、停止或清理已运行的其它夹具；不改变 VM、swap、分片上限或普通测试包并发；不安装依赖。

## 关闭

登记后立即并入 `ACCEPT-HARNESS-01`，信号、实现与剩余验收在主任务继续；本条关闭不表示验收通过。红绿及资源证据保留在 `.validation/runs/20261016-auto-opensearch-lease/`。
