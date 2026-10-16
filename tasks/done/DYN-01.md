# DYN-01 · 动态 State 的真实部署与重启恢复

- [ ] 按投影控制的持续服务与恢复合同完成整条部署验收。所属阶段：M4；承接原动态索引未完成项。

**现状与影响：** State READ、动态投影、变化通知及同 basis 回读均有实现和局部验证。缺的是独立 Observer、runtime、lakeFS、KC 与 OpenSearch 串联后的完整证据，以及 DOC-20 所列消费闭环缺口；已有双容器/adapter 测试不能证明 KC 重启后整条链仍能继续运行。

**要完成什么：** 用真实源变化触发 Observer，服务经已授权 Binding 拉取，推进动态投影；稳定知识仍由 Collector 走 Writer。KC 不直接读源夹具，runtime 不与 KC 共享内存 fake，Collector 不直写索引，观察值不写进 Snapshot。

**完成标准：** 源值改变后无需消费者触发维护即可检索到新观察；Snapshot commit 和文件视图不变；KC 替换/重启后能够恢复并继续追赶；重复、乱序、失败通知有后态证据，分页不混版本，回读失败不伪装为零命中。通知未送达时的时效/失联行为按 REVIEW-04 裁决验收，不能顺带声称实时完整。

依据：[投影控制](../../doc/design/discovery.md) 的服务目标、通知对账与方向性用例、[动态物化](../../doc/design/liveaccess.md)、[现有索引合同](../../discovery/projection/README.md)。这里验收 State，不把 Stream 窗口或多实例当作已经完成。

## 关闭

2026-10-15 拆分 `TASK.md` 时关闭，未按原条目验收。里程碑方向，已登记在 [路线](../../doc/roadmap.md) 的“未到”，立项时按 `tasks/` 写法重开。
