# DEBT-DEPLOY-LAKEFS-MEMORY-01 · 本地 lakeFS 采用实测软预算并恢复异常退出

## 债

- 位置：`scripts/deploy/compose.yaml` 的 lakeFS 运行环境与重启策略。
- 信号：历史 lakeFS OOMKilled 后因 restart=no 长期离线；恢复后真实验收前态使 Go managed memory 增至 1,721,494,424 B，运行时软上限仍为 MaxInt64，Docker VM swap 接近耗尽。
- 判定：本地共享 VM 内存有限，默认 GC 按比例扩张且无软预算；一次故障又缺恢复策略。初始低负载 RSS 不能证明验收容量足够。
- 处置：登记后立即并入共用真实部署的 `ACCEPT-HARNESS-01`。按实测空闲堆与在用内存设置 1536MiB 运行时软预算，异常退出有限重试；保留普通 GC、缓存、权威卷和系统限制。
- 触发条件：`ACCEPT-HARNESS-01` 当前真实前态运行已经触发可观测内存增长，须继续在同一部署复跑核验。

## 目标

本地 lakeFS 在实测软预算下完成真实验收，异常退出可有限恢复；原权威、bootstrap 一次性语义和系统资源限制保持原合同。

## 依据

- [服务](../../doc/design/service.md)
- [测试与验证](../../doc/engineering/testing.md)

## 验收

配置合同先红后绿，local/dev 合并配置保留软预算与各自重启策略；真实复跑核对实际 gomemlimit、内存/GC/VM 余量与 OOM 状态，统一文档/质量门禁通过。

## 前置

并入 `ACCEPT-HARNESS-01`，复用其真实部署与最终冻结门禁。

## 不做

不提高 VM、kernel、swap 或分片限制，不调整 lakeFS 缓存，不删除或重置权威数据，不以软预算宣称 RSS 硬保证。

## 关闭

登记后立即并入 `ACCEPT-HARNESS-01`。配置红绿、预算计算与只读证据在 `.validation/runs/20261016-lakefs-oom-recovery/`；实际复跑与统一门禁由主任务继续完成，未将配置通过写成部署容量通过。
