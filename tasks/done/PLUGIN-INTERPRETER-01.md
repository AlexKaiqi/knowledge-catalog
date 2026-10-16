# PLUGIN-INTERPRETER-01 · 插件按声明形状认领，不绑定 Dataset 或仓

## 目标

插件装在部署里，按形状认领声明：采用 `kc/index-declaration` Schema 的方面归声明式索引，单元上的
`valueSource` 归 LiveAccess。部署把能力清单名对应到唯一的实例，LiveAccess 用取值描述的 `runtime` 与
`protocol` 作为清单名。没装插件时声明照常读写，只是明确报没有能力。代码里不再有按仓选择插件。

## 依据

- [插件](../../doc/design/plugins.md)：规则、范围与关闭。
- [发现](../../doc/design/discovery.md)、[LiveAccess](../../doc/design/liveaccess.md)。

## 验收

- [accept-01-interpretation](../../.data/scenes/service-initialized/named-repositories-created/qinghe-object-published/_specs/accept-01-interpretation.yaml)、[accept-11-retirement](../../.data/scenes/service-initialized/named-repositories-created/qinghe-object-published/_specs/accept-11-retirement.yaml) 全绿。
- 删除 `discovery/adoption/config.go` 与 `discovery/plugin/config.go` 中的 `SelectRepositoryPlugins`；
  部署配置里还写着按仓选择插件，或同一清单名对应多个实例时，部署拒绝启动。
- 架构不变量表随反例测试一起更新受影响的行。
- `make check-docs`、`make quality`、`make test` 通过。

## 替换的旧用例

规则：旧用例在删除对应行为的同一批改动里改写或删除，不提前删；改写时保留原用例防的风险，删除时写明由哪条新规格接住。全部处理完后 `accept-retire-*` 结构检查变绿。路径里 `RA` 指 `.data/scenes/service-initialized/source-repositories-configured/repository-attached`，`QH` 指 `.data/scenes/service-initialized/named-repositories-created/qinghe-object-published`。

- Go：`discovery/adoption/config_test.go` 的 `TestSelectRepositoryPluginsPreservesTypedAdoptionAndSourceFaults`、`discovery/plugin/plugin_config_test.go` 的 `TestRepositoryPluginConfigurationDistinguishesMissingDisabledAndInvalid` 删除，由 `accept-interp-*` 中“部署拒绝启动”两条接住。
- Go：`internal/arch/plugin_adoption_test.go`、`plugin_adoption_guard_test.go`、`liveaccess/fixture/adoption*.go`、`cli/scene_repository_adoption_test.go`、`cli/fixture_index_config_test.go` 里按仓采用插件的断言，改写为按形状认领与清单名对应实例；守卫只能改写为等价或更严的断言。
- 场景：`QH/_meta.yaml` 的 `runtime.accessors.repository` 与 `.data/scenes/goto.py` 的对应分支，核对是否只是测试源的通知目标；若承担了按仓选实例，改为部署配置。
- 结构检查：`accept-retire-repository-plugin-selection`。

## 前置

`ACCEPT-HARNESS-01`。从当前代码出发按新设计做，不沿用已中止的 `REFACTOR-OBJECT-MODEL-01` 里按旧目标写的步骤。

## 不做

不设计插件之间的优先级和覆盖。

## 实施记录（2026-10-15）

前置 `ACCEPT-HARNESS-01` 按用户指示暂缓；`accept-*` 规格因此没有执行器，其中可在 Go 里证伪的条目已落成下列测试，其余等执行器。

已改：部署顶层 `plugins`（严格解码，`stores.yaml` 同名字段）；`PluginsConfig.CheckCatalogs` 是清单名唯一的唯一定义处；
`OpenIndexPlugins` / `liveaccess/service.OpenPlugins` 只读部署配置；残留的 `repositories[].plugins` 部署拒绝启动；
`SelectRepositoryPlugins`、`RepositoryPluginsKey`、`RepositoryPluginConfig(s)`、`ParseRepositoryPlugins`、`liveaccess/fixture` 删除。

决定：

- 实例配置不支持 `endpoint` 字段（按严格解码作为未知字段拒绝）；物理地址仍由 `stores.opensearch` 与取值描述给出，设计没有要求实例自带地址。`QH/.../variants/deployment/duplicate-catalog-mapping.yaml` 已去掉该字段。
- 在声明按清单名路由（`INDEX-SEARCH-SPACE-01`）之前，唯一安装的索引实例解释部署全部来源，多于一个索引实例拒绝启动，不猜测。
- 共享实例下发现真实回归：`CatchUpRepository` 会跑同实例其它来源的维护。修复为 `Controller.CatchUpRepository` 按仓追平（`demandRepositories` / `reconcile` / `catchUpState` 都带 `only`），由 `TestIndexPluginsSyncOneSourceDoesNotRunAnotherSourcesMaintenance` 证伪；两个测试包装器同步补了该方法。
- `VerifyIndexPlugins` 保留：冷装配之后连接恢复可能增加来源，激活前按当时全部来源重开一次实例；不再读取任何来源配置。它不是冗余重复，不登记为债。

旧用例去向：

- `discovery/adoption`、`discovery/plugin` 的按仓配置解析用例删除，由 `home/deployment_plugins_test.go` 的 `TestDeploymentRefusesRetiredPerRepositoryPluginSelection`、`TestDeploymentRefusesAmbiguousOrUnknownPluginInstances`、`TestDeploymentRefusesOverlappingAccessRuntimes`、`TestDeploymentWithoutPluginInstancesExplainsMissingCapability` 接住。
- `declarativeindex/plugin` 的按仓元数据失败用例（operations、unknown entry）改写为 `TestIndexPluginsSyncOneSourceDoesNotRunAnotherSourcesMaintenance`、`TestIndexPluginsUncompiledEntryInstallsNoCapabilityAndKeepsFixedReads`；元数据读失败这一风险随按仓配置一起消失。
- `home` 的 `KeepsConfigurationFailuresRequired` 与旧 connected runtime 用例改写为 `TestConnectedPluginRuntime*` 三条：冷恢复与激活都不读来源 metadata。Dataset 准备失败的原子性由 `discovery/datasets` 的准备用例覆盖，`home/dataset_plugin_preparation_test.go` 只保留装配侧两条。
- `cli/scene_repository_adoption_test.go` 改写为 `TestSceneRepositoryFixtureCarriesNoPluginSelection`（更严：退役后缀解析即失败）；`cli/fixture_index_config_test.go` 与 `adoptLiveAccessFixture` 删除，夹具依赖部署编译默认；`seedUnindexedRepo` 与 `seedRepo` 合并。
- 场景 DSL `Given existing repository <id> with LiveAccess instance <name>` 退役：`goto.py` 与 Go 场景执行器都拒绝该后缀，`test_goto_probe.py` 改为断言拒绝且不执行构建。`QH/_meta.yaml` 的 `runtime.accessors.repository` 只是测试源通知目标，不选实例，保持不变。
- `accept-retire-repository-plugin-selection` 升级为自动守卫 `internal/arch/plugin_retirement_test.go`（含反例），登记为不变量 `PL-01`。

基线收紧：删除 `liveaccess/fixture` 后各单元 Ce/Ca/传递可达只减不增（`discovery/adoption` Ca 6→5、`kernel` Ca 55→54、`snapshot` Ca 21→20），但单元数 77→76 使 propagation 分母变小，比例从 14.83% 升到 15.14%。按新结构执行 `go run ./scripts/coupling -write-baseline`，不是放松耦合。
