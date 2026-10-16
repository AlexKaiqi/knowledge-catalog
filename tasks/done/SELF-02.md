# SELF-02 · Gitea 与 lakeFS 托管 Snapshot Store 自动供给

- [ ] 用户登录后只需创建 Repository，后台自动完成 Store 账号/租户和仓的分配、绑定与恢复。所属阶段：M3；这是现有底层 managed create 之上的产品闭环。

**目标流程：** `kaiqidong` 登录 KC，只选择创建 Repository、用户可理解的仓名和允许的托管 Store（Gitea 或 lakeFS）；KC 后台从当前用户与产品上下文生成完整 Repository identity 和幂等命令身份，确保该 Store 中存在 identifier 为 `kaiqidong` 的账号或隔离租户，创建仓、保存内部连接、登记 Catalog，并按策略授予创建者维护能力。用户不填写完整 Catalog/Repository 协议坐标，不接触 command-id、Store 地址、服务账号、token、数据库目录或内部 allocation id。

**已有实现与待验收范围：** 按名称建仓、分配账、binding、创建者授权、管理地址与重试恢复已有实现；Gitea 同名账号供给仍需已配置的登录源。当前托管目标为 Gitea 与 lakeFS，Dolt 已退役。当时 WALK-01 的供给结果仅保留为历史范围，不能由替换后端名称外推 lakeFS 的全部隔离、故障与恢复资格；本项继续按下面的完成标准取得当前部署证据。

**设计边界：** KC 用户名是跨 Store 的产品 identifier；Gitea/lakeFS 的内部 subject、账号 ID、仓 ID、DSN、凭证和目录只保存在 Store Directory/供给账。Repository identity 不随选择 Gitea 或 lakeFS 而改变，也不能从物理仓名反推协议身份。Gitea 和 lakeFS 可以有不同的物理隔离机制，但对用户呈现相同的创建、状态、失败和恢复语义。

**需要补齐：** 托管 provider 的用户/租户 ensure、仓 allocate、权限安装、binding 持久化和幂等恢复状态；用户名冲突、已有同名账号归属、部分成功、重试、停用/改名、Store 不可用、配额及删除/归档规则。服务凭证只能代表平台执行供给，不能成为知识创建者身份。

**完成标准：** 全新的 `kaiqidong` 只登录 KC，输入仓名并选择 Gitea 或 lakeFS，即得到属于 `kaiqidong` 隔离空间的 Repository；随后直接发布和回读。创建返回用户名称、仓名、逻辑 Repository identity 和状态，两个 Store 的普通返回均不暴露物理账号 ID、allocation、路径、command-id 或服务凭证；重复点击和实例替换返回同一逻辑结果；另一个用户不能占用、打开或恢复 `kaiqidong` 的 Store 空间。任一分阶段失败都有可恢复后态，不产生无人认领账号或仓。

依据：[服务与托管仓设计](../../doc/design/service.md)、[Store 边界](../../doc/architecture.md)、[当前供给实现](../../home/managed.go)、[Gitea adapter 合同](../../snapshot/README.md)。

## 关闭

2026-10-15 拆分 `TASK.md` 时关闭，未按原条目验收。Gitea 已退役，托管仓只剩 lakeFS；托管仓供给已具备（见路线“已有”）。
