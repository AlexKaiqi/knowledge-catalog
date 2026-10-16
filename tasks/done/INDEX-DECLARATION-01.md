# INDEX-DECLARATION-01 · 索引声明与能力清单：Writer 校验、清单核对、试运行

## 目标

索引声明是采用 `kc/index-declaration` 的方面，每个成员描述一个检索字段（输入、构建方式、用途、
所依据的能力清单版本）。Writer 只查形状和定位符能否在所引用 Schema 上解析；插件实例公开带版本的
能力清单，按清单逐成员核对，清单外的写法标为不支持并说明原因。试运行和 Writer 对同一文件判断一致。

## 依据

- [发现](../../doc/design/discovery.md)：索引声明、能力清单。
- [Affordance](../../doc/design/affordance.md)：说明入口。
- [插件](../../doc/design/plugins.md)。

## 验收

- [accept-06-index-declaration](../../.data/scenes/service-initialized/named-repositories-created/qinghe-object-published/_specs/accept-06-index-declaration.yaml)、[accept-07-index-capability](../../.data/scenes/service-initialized/named-repositories-created/qinghe-object-published/_specs/accept-07-index-capability.yaml) 中归本任务的 5 条在测试部署上执行为绿：`accept-search-writer-rejects-index`、`accept-index-reject-shape`、`accept-index-cross-repo-schema-ref`、`accept-search-capability-catalog`、`accept-index-dry-run-matches-writer`。其余 6 条需 Dataset 内按成员准备、构建或归因，归 `INDEX-SEARCH-SPACE-01`；归属由 `.data/scenes/accept.py` 的 `DATASET_INDEX_SPECS` 登记、`accept_test.py` 锁定。
- `kc plugin declarative-index help` 与 `describe` 给出清单；清单不开放原始 mapping、脚本、分片。
- Schema 解释器删除字段 `access`，Writer 遇到旧写法拒绝（原重构 4a 剩余部分）；不变量 `S-01`、`K-26` 改写并附反例测试。
- `make check-docs`、`make quality`、`make test` 通过。

## 替换的旧用例

规则：旧用例在删除对应行为的同一批改动里改写或删除，不提前删；改写时保留原用例防的风险，删除时写明由哪条新规格接住。全部处理完后 `accept-retire-*` 结构检查变绿。路径里 `RA` 指 `.data/scenes/service-initialized/source-repositories-configured/repository-attached`，`QH` 指 `.data/scenes/service-initialized/named-repositories-created/qinghe-object-published`。

- 夹具：以下业务 Schema 去掉字段 `access`，检索能力改由 Dataset 维护者仓里的索引声明提供：
  `RA/_materials/published-objects/common/_schemas/published-object.definition.v1.aspect.yaml`、
  `RA/domain-schema-published/_materials/drafts/schema.metric.definition.yaml`、
  `RA/domain-schema-published/_materials/graph-drafts/schema.relation.yaml`、
  `QH/_materials/table-meta/drafts/` 下 10 份、`QH/_materials/sales-semantic/drafts/` 下 6 份。
  系统 Schema（`.data/scenes/_materials/system/` 与 `systemrepo` 两份副本）同步去掉。
- 场景：`RA/domain-schema-published/schema-read-granted/_probes/schema-describe-returns-access-fields.feature` 改写为“describe 只返回结构并指向索引声明说明”，风险由 `accept-retire-repository-scoped-search` 接住；`QH/_meta.yaml` 的 `publishes: schema.access` 改为索引声明。
- Go：`object/schema_access_test.go`、`object/reader/schema_access_scope_test.go`、`object/writer/schema_access_test.go`、`object/writer/schema_access_evolution_test.go`、`declarativeindex/projection/schema_field_access_test.go`、`schema_adoption_test.go`、`schema_adoption_live_test.go` 删除或改写为索引声明成员的等价断言；其余只是借 Schema `access` 搭夹具的测试（约 60 个文件，`grep -rlE 'access: \[|"access":' --include=*_test.go` 可列出）改用索引声明夹具，不改原断言目标。
- 结构检查：`accept-retire-repository-scoped-search` 的 Schema 部分。

## 前置

`PLUGIN-INTERPRETER-01`。从当前代码出发按新设计做，不沿用已中止的 `REFACTOR-OBJECT-MODEL-01` 里按旧目标写的步骤。

## 不做

系统不提出意图，不生成声明，不按字段类型推默认声明。

## 接管

判定时间：2026-10-15 20:46:45（Asia/Shanghai）。最后一次观察到进展：20:24:27，`discovery/service/dataset_observations_test.go`更新。
至判定连续22分17秒四项信号均未再出现：任务文件mtime未更新；排除我方路径后，本任务范围无新文件变化；
HEAD仍为 `94383e50d1c5f8e99075acd3d6b02fabeef8819c`、无新提交；`ps`未发现非本轮的相关测试或构建。
逐次观察和我方路径排除清单在 `.validation/runs/20261015T120241Z-task-orchestration/`。
接管前任务全文、在途diff、工作区状态在 `.validation/runs/20261015T124645Z-index-takeover/`；已读任务目标/替换用例与当前改动，从现有代码继续，不回退、不重写已做实现。
实际目标和依据：发现设计的索引声明/能力清单，Affordance说明入口及插件解释器边界；只收口本任务实现与夹具，先跑受影响测试作为接管基线。设计未改，尚不推进INDEX-SEARCH-SPACE-01。

冲突记录（2026-10-15 21:25:58，Asia/Shanghai）：`discovery/service/dataset_observations_test.go`出现非本轮的新写入（本轮未获分工也从未修改该文件），mtime/hash已变化、HEAD未变。符合接管后原范围恢复写入条件，立即暂停本任务全部源码修改并停止相关子任务；未覆盖对方改动。证据 `.validation/runs/20261015T132558Z-index-takeover-conflict/`。后续归属已由用户决定（2026-10-15）：由本线完成本任务并移到 `done/`。上述 trial 缺陷已在收尾中修复，见下文“收尾”。

## 接管后实现与定向证据

- 接管先跑基线：`.validation/runs/20261015T124726Z-9d07bcb66d3e/report.json`，621个通过事件、26个失败、0 test skip，源码未变。其中Dataset规划4项仍用旧夹具；projection22项因未注入真实OpenSearch未到断言。
- 使用已缓存镜像新建本轮独占OpenSearch验证容器（不是local部署或共享OpenSearch），注入真实引擎后的基线为237 pass、19 fail、0 test skip，源码未变，`.validation/runs/20261015T125648Z-628c6f1e3402/report.json`。后续仅清理本轮容器，不重置既有环境。
- 续改Dataset与OpenSearch旧夹具，保留字段选择/权限、两仓排序、摘要变化、Schema结构破坏拒绝，以及整数/纳秒/分页/命名空间隔离断言。Schema排除的计数测试把声明挂在既有方面对象上；命名空间测试新增独立声明对象后，明确将它计入普通对象数。未排除Schema以外的对象或削弱隔离断言。
- Dataset规划经维护保留的真实成员见证，在Dataset固定范围精确复核；新增“只选Schema、未选声明”反例，且让返回仓级原合同的错误实现明确变红。两仓声明使用不同ObjectID，避免依赖已知的重复方面准入缺陷。证据 `.validation/runs/20261015T125810Z-indexplan-fixtures/notes.txt`；216项全包绿在最后一次夹具身份修正前，该修正后6项定向绿，不混报同一次运行。
- 新增Writer声明回归覆盖多输入同批草稿、定位缺失/形状/原始物理配置拒绝、清单外合法写法、业务index名称、固定Schema、远仓唯一/歧义/显式定位、继承schema_ref。只读`CheckIndexDeclaration`共用Commit的结构和Schema判断，返回按inputs顺序解析的字段；不触Snapshot/账本/evidence。它只检查声明写法，不承诺路径准入或未来CAS；同basis路径占用的独立反例保留在评审证据，不冒充完整Commit预检。
- 独立评审发现两个真实fail-open并当场修复：本地Schema临时读取失败不能变为缺失后跨仓回退；远仓HEAD故障不能跳过而错误判为唯一。正式Check/Commit故障回归在旧解析器先红（Commit曾错误推进HEAD）、修复后绿。证据 `.validation/runs/20261015T131011Z-index-check-review/`，相关42项通过、无skip。
- Writer/format/reader/systemrepo完整受影响四包395个通过事件、0 fail/skip，运行源码未变：`.validation/runs/20261015T131512Z-2869a5a4b3ed/report.json`。系统出版物仍声称Schema.access提供检索的两处错误由新增自动合同先红后修；相关README和场景失效具名测试引用同步，views/states/product结构检查通过（不表示规格已执行）。

实例清单、正式describe/trial传输与manifest物理摘要仍在本任务继续收口；未开始INDEX-SEARCH-SPACE-01。accept-06/07包含Dataset按成员构建、准备和归因，与后续任务的开工前置形成验收环；按授权依设计裁决，见下文“裁决”。本机真实部署预检Server503且live-origin缺失，不能将离线/定向绿写成真实部署规格通过。本任务继续doing，待源码稳定后的统一门禁和完整验收。未提交。

接管冲突续记（2026-10-15 22:02 +08:00）：五分钟观察又发现非本轮新增改动，包括 `cli/dataset_scene_scope_test.go` 与 `systemrepo/content/capabilities/*/entrypoints.yaml`。本线自 21:25:58 起继续暂停，未覆盖这些文件；此前稳定门禁只证明各自运行时的源码状态，不覆盖之后的新改动。归属随后由用户决定由本线完成，这些文件在收尾中按本任务目标继续修改；快照见 `.validation/runs/20261015T120241Z-task-orchestration/` 的 14:02 UTC 观察记录。

## 裁决

按已获授权，设计与守卫或验收顺序冲突时依设计裁决，不再提问：

- 夹具使用正式 ID：声明成员对象写 `index/<名称>`，Schema 写正式 `schema/...` ID，不为测试另起别名。
- 消费路径用见证：Dataset 规划经维护保留的真实成员见证复核，不让仓级合同代替。
- 系统仓随出版物带 17 个声明成员，计入系统对象数。
- 跨仓 Schema 输入在本任务判为 unsupported 并说明原因，Dataset 级编译交给 `INDEX-SEARCH-SPACE-01`。
- `PlanAccess` 默认只有结构；未选进声明时没有检索字段。`index/fixture` 等声明对象计入普通对象数。
- 无投影时 `RequiresState` 返回 false；固定版本按该版本的声明解释。
- accept-06/07 中依赖“Dataset 内按成员构建、准备和归因”的部分属 `INDEX-SEARCH-SPACE-01`（发现设计进度表已列为近期项）；本任务交付声明、Writer、清单、describe、trial 与旧 access 退役，不降低那部分验收，由后续任务承接。

## 收尾（2026-10-15，用户指示由本线完成）

- 评审缺陷修复，各配回归测试：trial 逐成员独立判断（`TestTrialJudgesEachMemberIndependently`）；trial 与 Writer 一样继承已存 `schema_ref`，缺失时 `USAGE_INVALID`（`TestTrialInheritsStoredSchemaRefLikeWriter`）；能力清单与 trial 路由拒绝任何查询串，含畸形和空 `?`（`TestPluginCapabilityRejectsAnyQueryIncludingMalformed`）；`kc plugin ... trial` 拒绝非 UTF-8 声明文件（`TestPluginTrialRefusesNonUTF8DeclarationFile`）。Writer 反例补 engine script、分片设置、身份替代三种。
- 不变量：`S-01` 改为“检索能力只来自 index 方面成员，Schema 不带 origin/binding/access”；`K-26` 改为“声明不含插件实例、引擎地址、原始 mapping、脚本、分片或身份替代”，各附反例证据。新增守卫 `internal/arch/schema_field_access_guard_test.go`：出版物与场景 Schema 不得含字段 `access`，含正反例。能力清单与帮助不出现 mapping/script/shard/replica/pipeline，`Closed` 必须列出这些项。
- 场景：probe 改名为 `schema-describe-returns-structure.feature`，断言 `access` 缺席；RA 补 `index/metric`、`index/published-object` 声明成员夹具；`dataset_scene_scope_test` 改用 `schema describe --dataset` 作证据并加未交付反例。压测 `smoke.py`/`ladder.py` 改用声明成员。关系 Schema 去掉 access 后，graph 仓补 `index/relation` 三个成员（relationType 的 text/filter、direction 的 filter），等价替代原 `access: [text, filter]`/`[filter]`。声明草稿加 `path_hint: indexes/<对象>/<成员>.yaml`，scene-set 定义补选 `indexes/metric` 与 `indexes/relation`：按设计声明只在选进 Dataset 后才对该 Dataset 检索生效，未选时 Dataset 检索按“成员无法满足 SEARCH”失败关闭。
- 测试计数类断言随本任务新增的两条审核路由（capabilities、declarations:trial）同步：`httpsurface` 由 89 改 91 并登记两条具体 pattern；`validation-inventory` server 由 61 改 63。server 两个夹具补声明成员，对象数按“声明计入普通对象”改 3。重新生成 `systemrepo/content/capabilities/*/entrypoints.yaml`；生成物同时带入并行任务已改的帮助文案。
- 拆包与耦合：插件静态 Affordance 与能力清单从 `declarativeindex/cli` 移到新包 `declarativeindex/capability`；`discovery/cli` 改用 `discovery/client` 的类型别名，去掉对 `kc/discovery` 的依赖。原 `declarativeindex/cli` 的层、公开深度、插件采纳、运行时隔离、LiveAccess 归属等守卫逐条镜像到新包，原规则不放松，另加“旧包复制 PluginAffordance 被拒绝”反例。传播成本 14.26% 降到 14.01%；`declarativeindex/cli` 不稳定度 0.60→0.67 来自依赖方移走，不是新增依赖。已执行 `go run ./scripts/coupling -write-baseline` 显式收紧基线到 14.01%。

已验证：`make check-docs`、`make quality`（耦合棘轮 held）、`go vet ./...`；`internal/arch`、`declarativeindex/...`、`discovery/...`、`home`、`server`、`httpsurface`、`systemrepo`、`object/writer`、`liveaccess/handler`、`scripts/...`、`cli` 中不依赖 OpenSearch 的用例通过，无 skip。

`make test`（临时 OpenSearch）：前三轮依次暴露 graph 仓缺关系声明、scene-set 未选声明、声明落在仓根无法按目录选取，均已按上条修正；修正后的探查确认 Dataset 内可读到两组声明。最后一轮 `make test` 因执行确认超时未能运行，需补跑。

未验证或不属本任务的失败：最后一轮 `make test` 未跑完，OpenSearch 相关用例在最终源码上未验证；`internal/arch` 的 `TestObjectIsTheOnlyNameForCatalogContent` 来自 `ZK-TRIAL-01` 与动画目录命名；`snapshot/service` 与 `cli` 的库存用例属 `DEBT-INVENTORY-CONSUMPTION-01`；真实部署验收未跑。未提交。

## 补跑（2026-10-16）

- 首轮 `make test` 红于 `TestMetricPermissionScenes/dataset-query-principals-granted`：probe `dataset-search-follows-published-revision.feature` 重新发布的修订 2 只选了对象、关系和 Schema，漏选 `indexes/metric`、`indexes/relation` 两组声明，修订 2 上的 `kc search --dataset scene-set --query merchandise` 按设计拒绝（`workspace member kr://scene/graph cannot satisfy SEARCH: MATCH needs text access`）。修订 2 补选两组声明并断言 `sources.4/5.subPath`，与修订 1 一致；未改产品代码，未放宽断言。证据：`.validation/runs/20261016T012146Z-286dcee48066/`（红）。
- 第二轮 7 处 `write _results: rename ... no such file` 来自另一条线 09:30:30 同时启动的 `make test`（`.validation/runs/20261016T013030Z-8bd6849a753f/`）争写同一 `_results/latest.json`，场景断言均通过；无并发时重跑。
- 同一候选：`make test` 退出 0、无 skip（`.validation/runs/20261016T013220Z-ddfc32910d91/`），`make quality`（耦合棘轮 held）、`make check-docs` 通过。
- 验收第一条未达成：`accept.py --list` 中 accept-06/07 共 11 条规格全部 `not_evaluated`，3 条已有 probe、8 条无 probe（执行器归 `ACCEPT-HARNESS-01`）；`make deploy-local-accept` 未运行，真实测试部署此时由 `ACCEPT-HARNESS-01` 使用。其中按“裁决”属 Dataset 内按成员构建、准备和归因的部分须 `INDEX-SEARCH-SPACE-01` 才能变绿，与其以本任务为前置构成验收环，待用户裁决。本任务继续 doing。未提交。

## 验收拆分（2026-10-16，用户同意）

- 原第一条与 `INDEX-SEARCH-SPACE-01` 互为前置，构成验收环。按“裁决”最后一条拆开：本任务保留声明、Writer、清单、试运行 5 条；`accept-index-writer-accepts-unsupported`、`accept-index-array-input`、`accept-search-unsupported-member-isolated`、`accept-search-vector-member`、`accept-index-catalog-version-ahead`、`accept-index-catalog-upgrade-recheck` 6 条移入 `INDEX-SEARCH-SPACE-01` 验收，其中涉及试运行的断言随规格一并在那里验证。规格总数 117 不变，未删改任何规格内容。
- 执行器：`owners()` 增加 `DATASET_INDEX_SPECS`，新增 `test_index_declaration_specs_split_at_dataset_preparation`；`.venv/bin/python .data/scenes/accept_test.py` 16 条通过。
- 剩余：5 条中 `accept-search-capability-catalog`、`accept-index-dry-run-matches-writer` 尚无 probe（归 `ACCEPT-HARNESS-01`）；测试部署 `http://127.0.0.1:7393/readyz` 09:40 返回 503 `REGISTRY_STATE_UNAVAILABLE`，5 条均未执行。本任务继续 doing。未提交。

## 并入（2026-10-16）

`DEBT-INDEX-TRIAL-CONSISTENCY-01` 并入本任务：它是本任务目标“试运行和 Writer 对同一文件判断一致”的未达部分，与本任务改同一批文件，不另开线。原债的判定与调查过程保留在 `done/` 的该条。

- 位置：`home/index_trial.go`、`server/plugin_capabilities.go`、`discovery/cli/plugin.go`。
- 信号：2026-10-15 接管增量评审复现四项问题：同批坏成员连带污染合法成员的状态；更新文件省略 `schema_ref` 时尚未继承就拒绝；畸形非空 RawQuery 被解析器丢弃后放行；非法 UTF-8 原文件经 JSON 替换字符后改变合法性判断。没有证明凭证或私有 Schema 泄漏。
- 处置：红例入库；复用 Writer 有效 Schema 判断，分离成员上下文，拒绝所有非空 query 及无法无损表示的文件；不改 Writer 形状规则，不降低原断言。
- 红例与报告：`.validation/runs/20261015T133000Z-plugin-capability-review/findings.md`；执行报告 `.validation/runs/20261015T132424Z-94cb10afa5d3/`、`.validation/runs/20261015T132529Z-5749abb085cb/`、`.validation/runs/20261015T132538Z-cli-transport-review/`。

追加验收（与上文“验收”并列，未完成前本任务不移 `done/`）：

- 四项反例分别先红再绿，不以未装解释器代替结构判断，也不让一个成员的错误位置成为另一个成员的原因。
- 混合合法/非法成员、已有声明省略 `schema_ref` 更新、非法百分号 query 与非法 UTF-8 文件均有真实入口回归。
- 保留现有权限、无主体、固定 basis、无 worker 与 HEAD 或索引副作用断言。
- 复核正式 HTTP 闭集覆盖新端点，不只调整路由计数；另核对自定义 Manifest 数值 option 类型与 `checkOption` 分支是否一致（只读发现的待证线索，不预设缺陷）。

追加不做：不借此提前做 Dataset 检索空间，不新增登录方式或放宽授权。

## 并入补充（2026-10-16，L2 只读核对）

- `DEBT-INDEX-MANIFEST-NUMERIC-01`：`declarativeindex/capability/manifest.go` 的自定义 Manifest 选项若公开为 `number` 或 `integer`，`checkOption` 仍按非空字符串判断；只读反例覆盖公开 JSON 往返后拒绝数值、接受字符串。默认清单没有开放此类构建选项，不推断默认路径受影响。剩余验收为类型检查红绿及受影响能力合同；该文件不属本轮 L2 三个入口范围，未改代码。原债按归并关闭，证据 `.validation/runs/20261016T015959Z-l2-index-trial/manifest_numeric_review_test.go`。
- `DEBT-INDEX-PLUGIN-CLI-DOCS-01`：`discovery/cli/README.md` 仍称 plugin task 仅接受 help，遗漏已有 describe/trial；并入本任务说明入口文档收尾，剩余为按公开代码修正文案、重读和文档门禁。原债按归并关闭，本轮未越界修改 README。

L2 证据（2026-10-16）：`.validation/runs/20261016T015959Z-l2-index-trial/index.json`；既有四处生产修复未改，增强 home 已装解释器、HTTP 混合成员/继承 Schema、两路由畸形查询及真实 CLI/SDK UTF-8 原材料回归，逐项旧故障在独占快照红、当前定向 17 pass / 0 fail / 0 skip；HTTP 闭集双向对账及原权限/无主体/固定 basis/无维护与 HEAD 副作用断言绿，`make check-docs` 绿，`make quality` 停于主线保护文件 `discovery/service/dataset_search.go` 709 > 700 行（未改）。剩余为两条新并入项、主线门禁及 5 条真实部署验收；继续 `doing/`，未提交。

L2 扩展交付（2026-10-16，获准处理非主线文件后）：同一证据索引补记两条并入项已实现——`declarativeindex/capability/manifest.go` 按 number/integer 检查原始 JSON 数值并精确比较数值枚举，拒绝字符串和非有限浮点，不扩张默认清单；`manifest_numeric_test.go` 的公开 JSON 往返、超出浮点精度的整数/小数、科学计数、极端指数和错误类型先红后绿；`discovery/cli/README.md` 的 plugin 段补齐 describe/trial、固定依据与无写入边界，已重读。最终快照能力包 65 pass、入口合同 17 pass、完整架构组 649 pass，均 0 fail / 0 skip；文档门禁通过，克隆与耦合门禁单独通过，`make quality` 仍仅停于上述主线 709 行文件，未改上限。两条原债的归并历史保留；当前仍余主线完整门禁和 5 条真实部署验收，未移状态、未提交。

## 并入：`DEBT-INDEX-DEFAULT-CAPABILITY-01`

真实部署已恢复，2026-10-16 五条规格首次全部执行为 3 pass / 2 fail / 0 error；其中 shape probe 的错误文案匹配已修为原因及成员位置检查，待复跑。能力清单规格的默认 `metric-text`（模板）与 `metric-unit`（规范化）确实 unsupported，另两默认成员 supported；原规格/默认声明保留。位置为实例清单与实际索引构建选项，剩余验收为实现默认选项并跑完原五条部署规格与真实构建回归；详见 `.validation/runs/20261016-environment-recovery/capability-diagnostics/index.json` 和该目录验收报告。本任务仍 doing，未把执行成功写成验收通过。

## 默认能力实际构建补齐（2026-10-16，优先级并行修复）

- 默认 `metric-text` 除模板外还显式声明 `missing: skip`，旧清单同时拒绝这两项；`metric-unit` 的 `lowercase` 也未执行。先新增能力与字段构建反例，三项能力及模板/缺值反例按旧实现均红，见 `.validation/runs/20261016-default-index-builds/capability-red.txt`、`build-red.txt`。
- 保存 ready 声明的私有构建元数据，按成员组合文本并展开输入字段占位符；未知、畸形、同名歧义占位符明确不支持。keyword 的 lowercase 在值构建与查询两边一致；skip/exclude 只影响对应成员，不撤下其他成员。numeric keyword 真实按规范标量转字符串，保持 number 与 keyword 并存；等值有两可用成员时明确报歧义，范围只选 number，物理字段含成员身份以防配置串用。
- 能力清单只公开已实现的以上选项，保留 synonyms/prefix/boost/maxTokens/vector 等原拒绝；missing 公开实际可选值。声明状态补真实来源仓与固定提交作为归因坐标，纯来源坐标不进入构建摘要，避免普通对象变更强迫声明全重建。
- OpenSearch 的成员构建版本升为 `opensearch-v6-declaration-member-fields` / `object-units-v4-member-construction`，已有投影沿显式维护重建，不让旧字段键冒充新构建。Dataset 查询的可选源坐标保留到候选；缺一半坐标时失败关闭，组合对象保留内部固定基准并由上层按真实 Sources 回读。
- 已验证：能力包与 discovery 全包绿；OpenSearch 来源坐标、查询与 namespace 定向绿；真实已有 `http://127.0.0.1:19203` 上的 `TestOpenSearchDeclaredTemplateNormalizerAndMissingPolicies` 无 skip 通过，验证模板常量与输入次序、keyword 大小写、skip→exclude 重建且其他成员仍可过滤。证据 `targeted-green.txt`、`provider-green.txt`、`live-build-green.txt`。新真实回归只删除自身精确 repo-id generation 前缀与 control 文档；首次探查留下的两份本轮 generation 也已精确清理。
- 与 `INDEX-SEARCH-SPACE-01` 协作：成员编译、字段提取及物理查询在本线；Dataset 范围、准备与单空间打分在主线。最终统一门禁与原五条真实部署规格由根集中执行，本记录不代表这两入口验收通过，不移 `done/`、不提交。

### 显式向量成员与 HTTP 夹具收口

- 原 semantic HTTP 用例只发布了业务 Schema，靠环境配置把全部 text 隐式向量化。迁移保留其近似结果、最近邻次序、缺查询提供方不得降级的断言，改为显式选择 text 与 vector 两成员。
- 向量构建只处理成员声明的输入；多成员各自保留文本、模型与成员身份，使用 OpenSearch nested knn_vector，不把多个配置合并成一条对象正文向量。实例清单由与引擎同一配置验证器公开实际登记模型；未登记模型、未实现的距离继续拒绝。模型和维度进入物理摘要，防止接纳其他模型留下的旧向量。
- HTTP expression / search:rerank 用例补显式字段声明与 Dataset 维护同步，原布尔分组、排序翻页、固定依据、模型投影与归因断言保留；付费 live rerank 只迁夹具，未调用付费模型。
- 定向验证：`http-fixtures.txt` 的两条真实 HTTP 用例绿；`semantic-http.txt` 的语义 HTTP 用例与撤下 vector 后的反例绿，无 skip。撤下成员的变体用新 HTTP handler 读取新发布的 Dataset 修订并显式请求 revision 2，避免把原 handler 已读入的 revision 1 当成新修订；旧修订结果仍保留自身依据。能力包、discovery 全包及 provider 配置/来源/namespace/查询定向绿。
- 质量信号处理：模板、normalizer、missing 是同类“清单说不支持/实际编译未执行”信号，已提升为能力与真实构建回归；并存成员串用已当场修并由歧义/范围独立回归锁定；implicit text→vector 已当场修并由无 semantic 成员反例锁定。新 discovery/space 的公开深度登记缺口定向守卫报红，已交 `INDEX-SEARCH-SPACE-01` 负责修复，未放松守卫。

## 交付（2026-10-16，doing 统一补验）

- 最终冻结源 `547254bbd50b3062682da5489aa394167f734a9c5b766c6f5bf5bf0b4287ffa5` 在真实 lakeFS/Server/OpenSearch 部署上执行本任务原 5 条规格，全部通过、无 error/skip。清单、Trial 与 Writer、一致性反例及原退役检查沿原断言执行；逐条结果和实际运行程序摘要见声明验收记录（`.validation/runs/20261016-doing-validation/declaration-proof.json`）及其关联部署记录。
- 同候选 `make test` 18 pass / 0 fail / 0 skip，`make quality`（含 `make check-docs`）通过；完整合同三段全部执行，4818 pass / 0 fail / 11 个显式外部 live skip，各报告 `sourceChanged=false`。独立 live 补验及未验范围见本轮汇总（`.validation/runs/20261016-doing-validation/summary.json`）。
- 原 6 条 Dataset 成员构建规格继续归 `INDEX-SEARCH-SPACE-01`；117 条总数与规格原文不变。本条目标及指定验收达成，移入 `done/`；未提交、未安装依赖。
