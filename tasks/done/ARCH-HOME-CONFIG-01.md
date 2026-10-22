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

## 迁移清单（2026-10-21）

按「现 owner / 目标 owner / 生产消费者 / 是否本批」记录，审查依据是实际依赖而非文件归属。

| 现位置 | 目标 | 生产消费者 | 本批 |
|---|---|---|---|
| `home/deployment.go` 的 `DeploymentConfig`、`EvidenceConfig`、`RegistryBinding`、`RepositoryBinding` | `home/config` | `cli/deployment.go`、`cli/serve.go`、`server/serve_facade.go`、`scripts/fixture-deployment` | 是 |
| `home/deployment.go` 的 `ReadDeployment`、`Validate`、`validateSnapshotBindings`、`validateEvidence`、`Binding`、`String` | `home/config` | 同上 | 是 |
| `home/deployment.go` 的 `pathsOverlap`、`deploymentRealPath`、`rejectRepositoryPluginSelection`、`runtimeStores` | `home/config` | 根 Home 装配读取 `runtimeStores` 结果 | 是 |
| `home/admission.go` 的 `AdmissionConfig` 与校验 | `home/config` | 仅 `DeploymentConfig.Validate` | 是 |
| `home/repository_access.go` 的 `RepositoryAccess`、`validateRepositoryAccess`、`AuthenticatedRepositoryActions`、`ValidateAuthenticatedActions` | `home/config` | `DeploymentConfig.Validate`、server 授权判定 | 是（形状与纯校验；耐用文件读写留根包） |
| `home/dataset_access.go` 的 `DatasetAccess`、`validateDatasetAccess`、`AuthenticatedDatasetActions` | `home/config` | `DeploymentConfig.Validate` | 是（形状与纯校验；耐用文件读写留根包） |
| `home/managed.go` 的 `ManagedRepositoryConfig`、`validateManagedConfig`、`validateManagedPool`、`validateCreatorActions`、`validateShareActions` | `home/config` | `DeploymentConfig.Validate`；托管供给流程留根包 | 是（形状与纯校验） |
| `home/repository_id.go` 的 `NormalizeRepositoryID` | `home/config` | 配置校验与根包多处 | 是 |
| `home/stores.go`、`stores_profile.go`、`stores_file.go` 的形状与 `withDefaults`、`ValidateProfile`、`rejectSecrets` | `home/config` | 根 Home 装配、`scripts/fixture-deployment` | 是（见下方判断修正：`DeploymentConfig` 内嵌 `StoresFile`，不可拆开） |
| `home/deployment_runtime.go` 全部 | 留根包 | 显式初始化与恢复 | 否 |
| `home/managed_ledger.go`、`connections*.go`、`managed_named.go` | 留根包 | 托管供给与连接生命周期 | 否（归 `ARCH-HOME-REPOSITORIES-01`） |

## 实际迁移范围与判断修正（2026-10-21）

先前记为「`StoresFile` 本批不迁」的判断在执行中被证伪，此处按实际结果修正。

`DeploymentConfig` 直接内嵌 `StoresFile`，两者的配置形状不可拆开：只迁 `DeploymentConfig` 会把
`StoresFile` 留在根包，形成配置子包反向依赖根包的循环。因此本批整体迁移了 `StoresFile`、
`LayoutFile`、`HydrationCacheConfig` 与 `WithDefaults` / `ValidateProfile` / `RejectSecrets`。

对原判断的两个理由，实际处理如下：

- `StoresFile` 内嵌 `opensearch.Config`，迁入后 `home/config` 直接持有索引 provider 的配置形状。
  这是声明而非引擎：`home/config` 只命名部署要选的服务端参数，不构造索引实现。
  `internal/arch/declarative_index_module_test.go` 的实现边界因此把 `home/config` 与 `home`
  并列为装配点，理由写在该守卫的注释里；`TestCapabilityConsumerSets` 显式登记
  `home/config` 是 `identity` 与 `observability` 的消费者。
- `ValidateProfile` 原先调用 `authorityFor`，而该函数的驱动表同时承载 `open`、`openExisting`、
  `managedCreate`。本批把驱动表拆成两半：纯校验（`config.Authority` 的 `Validate` /
  `ManagedValidate`）留在 `home/config`，资源动作（`open`、`openExisting`、`managedCreate`、
  `stamp`、`discover`）只留在 `home/authority_drivers.go`。`ValidateProfile` 因此只依赖前者，
  配置子包不持有任何打开能力。

`home/config/deployment.go` 是唯一登记的内置认证器装配根：`Validate` 用 `identity.Lookup`
判定 `--auth`，所以该文件必须链接 `kc/identity/builtin`，否则 `local` 与 `taihu` 会被拒绝。
`internal/arch/identity_authenticator_test.go` 的装配根列表已随之从 `home/deployment.go`
改为 `home/config/deployment.go`。

## 验证（2026-10-21）

四道门禁全部执行：

- `make check-docs`：PASS，`doc/` 下 21 篇均可从 `doc/README.md` 到达。
- `make quality`：通过。`units=86 propagation=13.69%`，与收紧后的基线 `13.69%` 一致，
  ratchet held（无未知单元、无 instability 漂移 ≥ 0.05）。
- `make test`：`EXIT=0`，0 FAIL、0 SKIP。
- `make test-contracts`：`EXIT=0`，0 FAIL、18 SKIP；e2e 公开命令覆盖
  `53/53 successful` 且 `53/53 meet risk-tiered boundary requirements`。
  分母由 54 降到 53 是因为 `kc dataset clone` 已按 `DEBT-DATASET-CLONE-RETIRED-01` 退役，
  不是漏测。

耦合基线收紧已按要求执行 `go run ./scripts/coupling -write-baseline`：新增单元 `home/config`
（`Ce=11 Ca=1 I=0.92`），传播率由 13.54% 变为 13.69%，`-strict` 复核 held。

新增回归测试：

- `home/config/deployment_static_test.go`：`TestReadAndValidateDeploymentNeverTouchesItsSources`
  用不可达 authority 的文档证明读取与校验不联系来源，且不创建 `stateDir`/`cacheDir`；
  `TestValidateAcceptsExactlyTheRegisteredAuthenticators` 固定「内置认证器被接受、
  未注册名被拒绝」，防止注册链接再次丢失。
- `home/config/admission_test.go`、`home/config/auth_test.go`：从根包迁入，断言原样保留。

## 顺带处置

- `cli` 帮助用例的计数失败命中 `DEBT-CLI-GLOBAL-TRANSPORT-01` 的触发条件，
  按该债记录的修复方向把计数归属收敛到用例自己声明的哨兵主机，
  其余请求转交回原 transport；正反断言与上限均未放宽。该债的「不再替换包级 transport」
  目标未达成，任务仍留在 `todo/`，处置记录已写入债文件。
- `declarativeindex/opensearch/internal/opensearchimpl/config.go` 存在此前遗留的
  `KC_TRACE_LEGACY_NS` 跟踪分支，其中一行超长导致 `gofmt` 门禁失败；本次只做格式化，
  未改动该分支行为。

## 交付（2026-10-21）

实现范围：新增 `home/config` 包（`deployment.go`、`stores.go`、`stores_file.go`、`drivers.go`、
`managed.go`、`access.go`、`dataset_access.go`、`connections.go`、`auth.go`、`repository_id.go`），
承接部署配置的形状、单文档解析与静态校验；`home/deployment.go` 改为 type alias 加委派，
删除根包六份重复实现（`stores.go`、`stores_profile.go`、`stores_file.go`、`admission.go`、
`repository_id.go`、`deployment_auth_logout.go`），其余 home 文件改为委派。

已验证范围：`make check-docs` PASS；`make quality` 通过，耦合 `units=86 propagation=13.69%`，
基线由 13.54% 显式收紧并 `-strict` held；`make test` 0 FAIL / 0 SKIP；
`make test-contracts` 0 FAIL / 18 SKIP，e2e `53/53 successful` 且 `53/53 meet risk-tiered
boundary requirements`。

18 项 SKIP 逐条核对均为既有环境分级项：`TestLiveWriterUploadThroughput`、`TestLiveApplyThroughput`、
`TestWriterUpdateScalesLinearly`、`TestScaleProfileRepoAddLakeFS` 为规模与吞吐；
`TestLiveTaihuAuthentication` 为太湖；`TestLiveLunaListwiseRerank`、`TestLiveHTTPSearchRerankWithLuna`
为 Luna 重排；`TestLiveSnapshot*`、`TestLiveBulkApplyAgainstRealLakeFS`、`TestLiveLakeFSSceneDFS`
为 lakeFS 独立部署；`TestLiveHTTP*`、`TestLiveCapturedSources*`、`TestSceneAccessorRefreshUpdatesSearchIndex`
为 HTTP state runtime 独立栈。按各自合同单独进行，不计入本项阻塞。

未提交。`home/` 其余拆分（`ARCH-HOME-REPOSITORIES-01`）仍受索引接口稳定交付点前置约束。
