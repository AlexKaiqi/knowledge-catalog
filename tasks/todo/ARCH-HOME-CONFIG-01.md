# ARCH-HOME-CONFIG-01 · 部署配置与运行装配分离

## 目标

`home/config/` 拥有生产部署配置的解析、静态校验和纯路径推导；读取配置不打开 authority、索引或后台维护。
`home/` 根包继续负责显式初始化、恢复和依赖装配，生产配置与测试夹具配置保持各自合同。

## 依据

- [服务](../../doc/design/service.md)、[插件](../../doc/design/plugins.md)、[权限](../../doc/design/permissions.md)。
- [架构](../../doc/architecture.md)、[架构不变量](../../doc/engineering/architecture-invariants.md)。
- 实施时核对 `home/README.md`、配置公开类型与 `scripts/fixture-deployment` 的已有接缝。

## 范围

迁移 `home/deployment.go` 中配置形状、解析与纯校验，按实际依赖归位相关配置校验辅助。
`deployment_runtime.go` 的资源创建与恢复保留在组合根；服务可用性、凭据有效性和远端绑定核验
不冒充静态配置校验。夹具的 `HomeFile`、stores profile 与生产部署合同分别核对，不合成宽松配置。
配置依赖所属能力的纯合同；`home/config` 不导入根 Home 或仓管理运行实现。

## 验收

- 现行有效配置、未知字段、单文档限制、目录重叠、凭证限制、认证方式与插件声明校验保持原结果；
  对现有拒绝配置仍拒绝，不新增隐式默认或自动修复。
- 用禁止资源访问的测试证明配置读取与校验不联系来源、不打开引擎、不写部署状态、不启动维护。
- 显式初始化与仅恢复的入口区分不变；已有配置和部署状态无需因包迁移重写。
- 配置子包不反向依赖 `home`、`home/repositories`、`server` 或 `invocation`；
  新包显式归层，具体 provider 仍只由合法装配点构造。
- CLI 部署入口、Server 配置消费和 fixture-deployment 使用统一配置合同；
  现有公开入口及夹具接缝守卫保留，不为搬迁扩大脚本可访问面。
- 执行 `ARCH-LAYOUT-01` 的共同实施与验收约定；同步配置、装配说明和实际依赖图。

## 前置

可穿插做，与会话任务没有功能依赖，也不作为 Object 或 Index 的前置。
在 `OBJECT-VERSIONING-01` 与 `INDEX-PHYSICAL-01` 的共同接口审查后，先确认配置迁移清单：
不涉及索引新增持久状态、恢复与生命周期配置的部分可以推进；涉及的部分须等索引任务交付
该批配置及回归后再迁，不能按旧配置拆完再重做。
与 Dataset 生命周期或插件任务重叠的配置同样按批次等候，不要求这些任务整项完成。
只有一条执行线时让位于 Object 基础和索引主线。

## 不做

不改变配置格式或部署默认值，不实现 Dataset 保留/配额配置，不重新设计插件配置，
不搬迁私有仓管理账本，不把夹具参数开放到生产部署。
