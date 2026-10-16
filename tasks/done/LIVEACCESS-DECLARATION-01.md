# LIVEACCESS-DECLARATION-01 · 取值声明收口：只认 descriptorRef，取值描述带 identity

## 目标

取值来源只写在方面或成员上、只引用取值描述，Schema 不再承载任何取值声明。取值描述的字段集合加入
`identity`（`per-caller` 默认、`shared`），系统 Schema 的两份副本一致。取值描述所在仓没选入 Dataset 时，
按单元引用的版本解析，且不进入消费者可见范围。

## 依据

- [LiveAccess](../../doc/design/liveaccess.md)：声明、规则。
- [对象](../../doc/design/object.md)：表示一致性、跨仓引用。

## 验收

- [accept-02-live-declaration](../../.data/scenes/service-initialized/named-repositories-created/qinghe-object-published/_specs/accept-02-live-declaration.yaml)、[accept-05-live-versioning](../../.data/scenes/service-initialized/named-repositories-created/qinghe-object-published/_specs/accept-05-live-versioning.yaml) 全绿；
  [accept-13-cross-cutting](../../.data/scenes/service-initialized/named-repositories-created/qinghe-object-published/_specs/accept-13-cross-cutting.yaml) 中 `accept-cross-descriptor-repo-not-selected` 变绿。
- `system/resource-descriptor.v1.aspect.yaml` 与 `liveaccess/contract/trusted/resource-descriptor.v1.aspect.yaml`
  都加入 `identity`，有守卫保证两份一致；夹具 `ops/warehouse-stats` 能通过 Writer。
- 不变量 `S-01`、`K-28` 按 `architecture-invariants.md` 第 4 节登记的方向改写，附反例测试。
- `test-catalog.md` 中 B-01、B-02、B-04、B-13、B-23 等行更新为新证据。

## 替换的旧用例

规则：旧用例在删除对应行为的同一批改动里改写或删除，不提前删；改写时保留原用例防的风险，删除时写明由哪条新规格接住。全部处理完后 `accept-retire-*` 结构检查变绿。路径里 `RA` 指 `.data/scenes/service-initialized/source-repositories-configured/repository-attached`，`QH` 指 `.data/scenes/service-initialized/named-repositories-created/qinghe-object-published`。

- 规格：`RA/_specs/liveaccess.yaml` 已按新设计写（实例由部署按协议对应、外部值进索引要求 shared），本条接 probe 时沿用原 ID，不另起一份。
- Go：仍从 Schema 解析 `origin` 或 `binding` 的测试（以 `accept-retire-bound-schema-path` 的结构检查为准逐个找出）改写为单元头 `valueSource` 写法。名字带 Bound 但测的是 `binding` 类取值来源的测试（如 `TestBoundReadFailsClosedWithoutStateRuntime`）属于新写法，保留。
- 结构检查：`accept-retire-bound-schema-path`。

## 前置

`PLUGIN-INTERPRETER-01`。原重构阶段 4b（`94383e5`）已完成 `descriptorRef` 唯一写法，本条
只做差量：`identity`、跨仓解析不进入可见范围、剩余规格。

## 不做

不加显式取值参数（设计里标为待确认）。

## 实施记录（2026-10-15）

前置 `ACCEPT-HARNESS-01` 按用户指示暂缓；`accept-*` 规格因此没有执行器，其中可在 Go 里证伪的条目已落成下列测试，其余等执行器。

已改：

- `identity`：两份 `resource-descriptor.v1` Schema 加入 `identity`，一致性由 `TestSystemPublicationMatchesCanonicalFilesAndSchemas` 守住；缺省 `per-caller`、未知值拒绝、解析结果带出由 `TestParseResourceDescriptorIdentityDefaultsToPerCaller`、`TestWriterPublishesDescriptorIdentityAndRejectsUnknownOnes`、`TestResolvedDeclarationCarriesDescriptorIdentity` 证伪。
- 跨仓取值描述（`accept-cross-descriptor-repo-not-selected`）：`Dataset.Dependencies` 随修订冻结，`ResolvedDataset` / 回放 / `DatasetPin` 带出；`Serving.FreezeDependencies` 在准备阶段只经 `bindings.index` 元数据发现绑定单元的 `descriptorRef`，范围内读不到时在非系统仓中找唯一持有者冻结其当时提交；`Serving.ReadDependency` 只在冻结提交上读；`ServingDescriptors` 范围内读不到时回退到依赖版本。`Preparation.Dependencies`（必填）→ `Preparation.Execute` 返回带依赖的候选 → `DatasetPublisher` / `snapshot/service` 只采纳依赖，SetID、修订、来源被改写即拒绝。证据：`TestDescriptorInUnselectedRepositoryResolvesAtFrozenDependency`、`TestDescriptorDependencyFreezeFailsWithoutSingleHolder`、`TestDatasetDependenciesFreezeWithRevisionAndStayOutOfScope`、`TestDatasetDependenciesRejectInvalidCoordinates`。
- 结构检查 `accept-retire-bound-schema-path` 升级为自动守卫 `internal/arch/value_source_declaration_test.go`（含反例）：非空 `ResolvedBinding` 只由 `liveaccess.ResolveDeclaration` 构造、它读单元 `ValueSource` 且不触碰 Schema；`SchemaDefinition` / `SchemaFieldDefinition` 无 `origin`/`binding`/`valueSource`/`descriptorRef`；系统发布的 Schema（`systemrepo/content`、`object/trusted`、`liveaccess/contract`、场景系统材料）无 Schema 级取值声明。
- 不变量：`S-01` 改写为“Schema 只描述结构”的取值声明一半并挂新守卫；`K-28` 写明取值描述按冻结版本（含依赖版本）解释；`D-01` 补依赖证据。`test-catalog` B-01、B-02、B-04、B-13、B-23 更新。

决定：

- PinID 把依赖纳入计算（依赖不同即不是同一服务版）；无依赖时保持历史值，已发布修订的 PinID 不变。
- `Preparation.Dependencies` 必填，未注入时 `CAPABILITY_UNSATISFIED`，不跳过：单来源也可能引用其它仓的取值描述，与“多来源才要求冲突检查”不同。独立 snapshot 服务没有 `PreparePublication`，纯文件发布不受影响。
- native 成员没有绑定元数据口，冻结时失败关闭；不为它加转发（生产中目前没有 native 实现，只有测试夹具）。待出现生产 native 实现时由其提供 `bindingReferenceCoordinates`。
- 发现绑定单元只读定位器元数据，不扫描快照；旧 `bindings.index` 缺 `Descriptor` 时报缺能力，须显式重建。
- 字段级 `access` 退出系统 Schema 归 `INDEX-DECLARATION-01`（`S-01` 另一半与 `K-26` 一起改写），本条守卫不检查 `access`。

旧用例去向：

- 搜索结果中仍从 Schema 解析 `origin`/`binding` 的 Go 测试为零：命中的都是拒绝用例（`TestWriterRejectsSchemaOrigin`、`TestValueSourceRejectsInlineAccessShape` 等），保留。
- `server/snapshot_recipe_adoption_test.go` 手工构造 `Home`，补上 `home.Open` 生产装配必有的依赖检查口（单文件来源，无对象可引用）。

验证：`internal/arch`（新守卫、目录与不变量守卫）、`object/...`、`liveaccess/...`、`snapshot/...`、`discovery/...`、`invocation/...`、`systemrepo/...`、`server`、`cli`、`home` 中不依赖 OpenSearch 的用例通过；`make check-docs`、`make quality` 通过（耦合基线不变）。本机没有可用的 OpenSearch，依赖 `KC_TEST_OPENSEARCH_URL` 的用例（含 `home` 的 `TestConnectedPluginRuntime*`、`TestDatasetPublication*`）未验证。`internal/arch` 的 `TestObjectIsTheOnlyNameForCatalogContent` 失败来自并行任务 `ZK-TRIAL-01` 的零知识夹具命名，不属本条。
