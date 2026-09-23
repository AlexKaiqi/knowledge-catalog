# 协议旅程场景

协议用例怎么组织、维护、执行、怎么写断言。覆盖格子（状态 × 操作、已定位/partial/gap）仍以 [`docs/reviewed/test-catalog.md`](../../docs/reviewed/test-catalog.md) 为准。架构不变量 ID 以 [`docs/reviewed/architecture-invariants.md`](../../docs/reviewed/architecture-invariants.md) 为准。清河茶铺走查夹具写在 `named-repositories-created` 叶上，不要另起独立数仓黑盒套件。

这不是检索应用的故事包，也不是 `cli/testdata/`。

## 1. 组织

状态节点必须是**已经被多个独立用例实际复用的构建前态**，包括经后续构建间接复用。`fixture` 写明本步实际交付的条件；目录和 feature 生成消费者关系，不另写消费者清单。独立 Go 测试使用自己的 setup，不能充当场景状态的消费者。没有真实构建的测试分组、读操作结果、只被紧接一个用例使用的临时后态，都归入已有状态的 probe 或 Go evidence。

验证用例声明从前态出发的操作、预期和独立失败风险。用例可以写入、授权、撤权或恢复；只有后态形成真实共享前置才提取节点。投影就绪、服务启动等也可以形成有效前态，不以是否改变权威知识数据判断。检查多个文件不足以证明复用：要确认各用例确实消费了新增条件，不能把参数解析用例挂上来凑数。

从父状态冻结的 home 副本上只跑本节点 construct。打开一个节点目录就能读懂当前状态、怎么进来、夹具在哪、从这里验证什么。状态名描述交付的前置条件，用例名描述动作与预期；避免含义不明确的 `ready` 或把 `visible`、`browsed` 一律当成状态。执行器不把 `_results/` 当 Oracle。

| 目录 | 含义 |
|---|---|
| 不以 `_` 开头的子目录 | **分叉**：后继状态。父目录就是前态 |
| `_meta.yaml` | 本节点自描述：fixture 事实、角色、layer、工程视图标签，以及具体断言验证的产品条目。不是树的第二份抄本 |
| `_bundles.yaml` | 从本节点起的时间局部任务（可选），引用构建或独立用例，不传递 probe 的临时结果 |
| `_build/` | 从父状态进入共享前态的可执行 construct；不为独立 Go 测试建立目录或参考 construct |
| `_materials/` | 这一步用的夹具。`kc writer commit --dir $materials/…` 或 `put --file $materials/…`。`$home` 是本趟 home。System Schema 例外 |
| `_probes/` | 从本状态出发的独立验证，临时后态不被其他用例或子状态继承 |
| `_results/` | 本节点上次验证的 `latest.json`。gitignore，不是分叉或 Oracle；不得跨节点拼成同次全部通过 |

公开命令闭集由测试钉在 construct / probe（或 `_meta.yaml` 点名的 go-test）上，不另写一份覆盖清单：

- 每条 `cliSurface` 命令必须在某个状态的 `When I run` 里出现，或该状态 `_meta.yaml` 点名的正式 go-test
- `serve` / `kcfs` 不在 `cliSurface`，由 `TestSceneCatalogCoversPublicProductSurfaces` 另钉
- 不要在场景目录放名为 `catalog.yaml` 的文件（那是产品 Catalog 登记表）
- 不要在场景根放 `sources.yaml`、`checklist.md`，也不要再手写一棵「覆盖对照」树。节点标签留在 `_meta.yaml`

整体视图（给人 / Agent）：`python3 .data/scenes/tree.py` 或 `--json`。目录只形成共享前态树；probe 与独立 Go 旅程挂在最近的相关宿主上，后者仍由自己的 setup 和 Oracle 证明。`--check-states --json` 显示每个前态的实际 probe 消费路径；拒绝无 construct、无 fixture 说明和仅有一个下游 probe 的目录。该必要条件不能替代对实际依赖的代码审查。

### 1.1 工程关注点视图

根 `_views.yaml` 的 `views` 定义工程视图名称、说明及可选显示入口；节点与每条 probe/Go evidence 的 `views` 在本节点 `_meta.yaml` 就近声明。用例标签不从节点继承：同一节点的安全用例和发布用例可以分别选中。目录是父子关系的唯一来源，视图不另存节点/边清单。

| 视图 | 关注内容 |
|---|---|
| `access-control` | 接入、身份与独立授权边界 |
| `publication` | 从 `repository-attached` 出发的知识发布与投影 |
| `consumption` | 发现、固定版本组合、检索读取与清河茶铺走查 |
| `lifecycle` | 治理维护、撤权、退役、归档和恢复 |

视图可以重叠、可以有多个分支。显示入口允许不从根开始；隐藏祖先仍是构建前置，JSON 保留前置和真实边界关系。连接选中节点所需的中间状态作为上下文显示，不把省略路径伪造为直接依赖。工程主视图并集必须覆盖全部共享前态节点、目录关系以及每条 feature/Go 证据；未知标签、无标签和未覆盖项由 `--check-views` 拒绝。该检查证明声明完整，不表示用例已执行通过。

```bash
python3 .data/scenes/tree.py --list-views
python3 .data/scenes/tree.py --view publication
python3 .data/scenes/tree.py --view access-control --from repository-attached
python3 .data/scenes/tree.py --probes-at repository-attached
python3 .data/scenes/tree.py --view consumption --json
python3 .data/scenes/tree.py --check-views
python3 .data/scenes/tree.py --check-states
```

视图用于阅读与验证范围定位；`goto.py` 接收明确状态，不能把含多个终态的视图当成唯一目的地。已有部署可显式加 `--probe <文件名>`，在构建前态后执行一个正向走查；它改变当前走查环境，不冻结该用例的结果为共享节点，也不是隔离批跑器。执行前检查整条构建路径和选中用例，遇到不支持的步骤或 `$last` 等变量即拒绝，不先改动环境。错误、授权隔离等用例仍由测试执行器运行。bundle 仍描述具体用户任务。

### 1.2 与产品文档对应的视图

从产品任务了解用例时，先看 `--family product`。`_views.yaml` 的 `product_documents` 只指定已在 `docs/graph/` 登记的 owner 文档 ID 和用例章节；工具按文档顺序提取稳定条目 ID 与标题，生成 `product/<文档 ID>/<条目 ID>` 视图。当前来源包括 `knowledge-product-schema` 的「8. 用例」和 `materialization` 的「9. 用例」；条目来自各 owner 正文，ID 在文档内稳定，不另抄任务清单。派生 `docs/product.html` 不作为关联的权威来源。

设计书写方向性任务与不可接受的结果，场景树承接具体前态、操作和断言。尚未实现的能力先记
明确缺口，不创建没有真实构建过程的状态或不能执行的 feature。动态 State 的独立 Go 证据继续
挂在 `repository-attached`，不因为增加设计用例就虚构一个动态服务已就绪的共享状态。

关联方向是**产品条目 → 具体验证与断言 → 所需状态与构建前置**。在 probe 或具名 Go evidence 上就近声明：

```yaml
verifies:
  - claim: "knowledge-product-schema#U2"
    detail: 同时含非法 type 与 access 的声明被拒绝，Repository HEAD 保持不变
```

`detail` 只描述该用例实际证明的范围。构建中已有明确后态断言时，可在节点上用 `construct_verifies` 按同样结构关联；仅有状态名称不能充当验证。普通节点级 `verifies`、手写产品视图成员标签和引用整个 Go 文件都不能替代具体验证。

产品视图由这些关联自动选中用例，补齐宿主与真实构建前置，不继承同节点其它 probe。一个用例可以关联多个产品条目。`--probes-at` 可反查用例对应的条目。文档之间的依赖、细化和验证关系仍只维护在 `docs/graph/`。

```bash
python3 .data/scenes/tree.py --family product
python3 .data/scenes/tree.py --view product/knowledge-product-schema/U2
python3 .data/scenes/tree.py --view product/knowledge-product-schema/U2 --from repository-attached --json
python3 .data/scenes/tree.py --view product/materialization/U3
python3 .data/scenes/tree.py --check-product
```

产品视图检查引用有效、断言说明非空，且每个文档条目都有验证入口或明确缺口。无入口的新条目、失效引用和伪造构建证据会报错。`product_documents` 的 `gaps` 只记录尚未由关联证据证明的具体范围，允许与局部证据并存，不保存节点或用例清单。

输出中的 `linked` 只表示已有关联，`gap` 表示已声明缺口；两者都不等于完整覆盖或测试通过。`execution_status: not_evaluated` 明确说明该工具没有执行用例。产品验收仍按 `docs/reviewed/test-catalog.md` 读取同次运行证据；工程并集与产品声明完整性分别检查。

具名 bundle 写在入口节点的 `_bundles.yaml`：声明从该既有状态开始的任务。执行器复用祖先夹具，用户步骤无需从初始化重复。

Catalog 固定 `kr://scene/catalog`。业务知识仓 `kr://scene/knowledge`、关系仓 `kr://scene/graph` 只给后续 `attach` 和「已配置、尚未登记」的探用，**不**在 `catalog-initialized` 进入登记表。`Given deployment fixture` 不创建业务 Snapshot。知识集 `scene-set` 发布时只纳入指定实体/关系路径，不是整仓别名。树自包含，不读数仓目录。

**树脊（接入方）**：`catalog-initialized` → `source-repositories-configured` → `repository-attached`（服务验证既有 authority 并原子登记）→ `domain-schema-published` → `semantic-knowledge-published` →（声明式索引）`projection-synced`。`repository-attached` 是接入完成；后续 commit、put 是知识发布。此脊验证外部既有 Snapshot 接入；部署启动和 attach 不隐式创建它。初始化前态挂载的托管建仓 Go 旅程使用正式配置，验证已授权接入方显式 create 平台仓、立即发布以及重部署后继续操作，目标仓无需静态配置。

树父是**构建前置**，不是权限蕴含：`knowledge-search-granted` 下的读授权用例复用已有检索条件，再验证额外授予 READ 后的变化；SEARCH 本身不授予 READ。

## 2. 维护

新增或改状态时：

1. 目录嵌套就是前态（只有一个父）。每个状态目录要有 `_meta.yaml`。公开命令必须落在某个 construct/probe 或点名的 go-test 里（测试钉闭集，不另抄清单）。
2. 每个目录都必须有 `fixture` 和 `_build/construct.feature`，`surface` 为 `feature` 或 `both`。construct 只做进入共享前态所需的步骤，再观测后态。独立 Go 证据写在相关状态的 `evidence` 中。
3. 夹具放在**构建或 probe 写入它的宿主节点**的 `_materials/`，不进 Catalog 登记表，不在场景根放 `materials:` 清单。
4. Domain Schema 草稿放在 `domain-schema-published/_materials/` 下按目标仓分目录：`drafts/` 属知识仓，`graph-drafts/` 属关系仓。construct 分别经 `kc writer commit --repo --dir` 发布并回读；引用 Schema 的实例必须在自己的目标仓找到该 Schema。实例与 note 用 `kc writer put --file $materials/…`，再 READ/browse 回读。不要用 `Given material` 代替这些公开命令。Gherkin 不出现 ChangeSet 文件。提交前对照用 `kc diff --repo --dir`，不是 construct 必经步骤。
5. System Schema：跟踪源是 `knowledge/system/schemas/`（`go:embed` 信任根）。`catalog-initialized/_materials/system/` 是旅程可见副本，**禁止** Writer PUT 到 `kr://kc/system`。改协议 Schema 先改跟踪源，再让副本与 `TestSceneSystemSchemaMaterialsMatchEmbed` 对齐。
6. 不要引用已删除的数仓黑盒套件。清河茶铺表/作业/语义/SQL 只写在走查叶 `_materials/`。接入方 Resource Access / Collector / Observer 的 Docker 与代码放在写入 origin 的节点 `_materials/accessor/`。
7. 不要在仓库根加 `tests/scenarios/` 或把协议场景拷进 `cli/testdata/scenes`。
8. 一条 feature 一个 Scenario。construct 与每个 probe 文件各一种失败风险。每条 probe 必须可以从本节点冻结前态单独执行；不得用文件排序传递写入、授权、登录会话或 HTTP 服务状态。有意连续的操作写在同一个用例里。
9. 观测走 `kc` / typed HTTP，不打开内部状态，不把 `_results/` 当断言。配置、权威数据、耐久控制状态、证据与可丢缓存分别维护。
10. `file.read`×Dataset / `dataset.resolve` / `retire` 的探走 scene 执行器（`--dataset` SEARCH/READ 是消费旅程，不是为绿而抄命令）。动态观察作为 `repository-attached` 的独立 Go evidence。成员仓解析的细 Oracle 仍可并列 go-test。

## 3. 执行

执行器 DFS `.data/scenes/`，按套件选择受支持的构建节点，**复用父节点 construct 之后冻结的前态**（测试 TempDir，gitignore 的 `_results/` 只记本次是否绿，不当断言）。在副本上只跑本节点 construct，再冻结本节点。每条可执行 `_probes` 从该冻结状态取得独立环境；子状态也从同一冻结状态构建，不继承任何 probe 的结果。父前态尚未冻结时（单节点 `-run`），才从祖先 construct 链重建。

`Given existing repository` 由执行器接到 lakeFS Snapshot adapter。Gherkin 不写介质名。默认 `make test` 运行 `TestProductScenes` 与 `TestMetricPermissionScenes`，使用进程内协议忠实假服务，不需要整套 lakeFS 部署：冻结和复制都必须隔离 lakeFS 物理仓，不能只拷指向同一远端的 home。HTTP 服务与客户端登录目录也按环境重新准备，不复用另一用例的 handler 或会话。OpenSearch 原生按逻辑仓标识投影，拷贝 home 不能隔离外部索引。测试执行器为每个环境提供独立物理索引与控制记录空间，仍由真实 OpenSearch 执行；复制环境重新绑定测试端点，关闭时只清理该环境自己的派生索引。需要已有投影时，在每条 probe 前重放构建链恢复基线；未构建投影的前态也不会继承其它用例的索引。场景仍顺序执行。独立 `make deploy-local-scenes` 在第一阶段部署拓扑运行同一棵树的 `TestLiveLakeFSSceneDFS`：真 Graveler + MinIO + PostgreSQL + OpenSearch，需先显式准备 local 测试栈。真 Graveler 不能按 commit id 分叉，所以 live DFS 的节点及各 probe 分别从祖先 construct 链重放，不克隆父 home。Catalog 组件夹具仍是 gitdir，不是生产 OpenSnapshotRegistry。

两条写入脊不要并成一条：`knowledge-published` 挂在 `repository-attached` 上（普通 Canonical）；`semantic-knowledge-published` 挂在 Domain Schema 发表之后（语义实例）。调整前置时改目录嵌套；`depends_on` 由工具从目录生成。

```bash
export PATH="$HOME/.local/go/bin:$PATH"
go test ./cli -run 'TestSceneCatalog|TestSceneFeaturesPinObservedState|TestSceneFeaturesCoverPublicCLI|TestSceneFeaturesCoverHelpShortestPaths|TestSceneConsumeJourneyIsOneFeature|TestSceneWriteSpine|TestSceneExecutorDiscovers|TestSceneGoTestFeatures|TestSceneExecutorReusesParentConstructHome'
go test ./cli -run 'TestProductScenes'                                   # 不构建检索投影、不跑动态 State
KC_TEST_OPENSEARCH_URL=http://127.0.0.1:19200 \
  go test ./cli -run 'TestMetricPermissionScenes'                        # 祖先含 projection-synced，或 construct 含 projection sync
go test ./cli -run 'TestProductScenes/source-repositories-configured$'           # 单节点
make deploy-local-scenes                                                 # 同一棵树，真 Graveler + OpenSearch
python3 .data/scenes/goto.py qinghe-knowledge-published --probe probe-publish-dataset-with-scoped-members.feature
make deploy-local-goto NODE=qinghe-knowledge-published PROBE=probe-publish-dataset-with-scoped-members.feature
```

- `TestProductScenes` 跳过需要索引或动态 State 的节点。
- `TestMetricPermissionScenes` 需要 OpenSearch（`make test` 复用 `KC_TEST_OPENSEARCH_URL`，未设置时自动启动一次性实例；手跑要自己设 URL），只运行当前夹具支持的索引节点；动态 State 与墙外走查由各自部署套件承接。
- `TestLiveLakeFSSceneDFS` 在 `KC_SCENE_LIVE_LAKEFS_URL` 上 DFS 全树；部署拓扑由 `make deploy-local-scenes` 注入 lakeFS / OpenSearch。动态观察的独立 Go 测试需要 State runtime；不再建立参考 feature 或跳过的场景节点。
- 走查 Server 用 `python3 .data/scenes/goto.py <id或目录>` 重放该节点祖先 construct。`catalog-initialized` 已由 `deploy-local-up` 初始化，脚本跳过它。`named-repositories-created` 分支需要托管 create 与墙外 compose，不进 `TestProductScenes`。
- `Agent as` 任务块给人 / KC-AGENT-01，**不是**协议 Oracle；协议绿看 Then。
- 定向 `go test` 只证明所选范围。默认 `make test` 只运行上述两组场景，不执行节点声明的独立 Go evidence；真实部署与完整合同组合分别由 `make deploy-local-scenes` 与 `make test-contracts` 承接，不能相互替代。

现场 ttyd 敲的是产品 `kc`，不展开 `$materials` / `$home` / `$last.id`。夹具用该节点 `_materials/` 的真实路径；`--id` 从刚才的 JSON 抄。不要 `export KC_AS`：它盖住 login 会话。live `deployment init` 会写入 bootstrap 管理主体，空 allow 的初始化前态只存在于 InitHome 夹具。

需要保留可复核的定向证据时，使用
`python3 scripts/validation.py run --scope scene-debug -- go test -json -count=1 -run '<选择式>' ./cli`。
它只运行显式选择，不自动补跑其它场景。正式 testsuite 自带同次运行目录；scene 同时保存
`scenes/<test>/<state>-<execution-id>.json`，包含 run-id、源码指纹、测试名、唯一执行身份、
开始/结束时间和观察步骤。同次 run 重跑相同 Test/node 也保留各次结果，不覆盖前次失败。
节点 `latest.json` 只是方便就近查看的副本；无 run-id 的直接运行文件不具有版本绑定能力。
完整库存由 `make validation-inventory` 读取覆盖表与场景目录生成；结果与 skip/未观测的区别
统一遵循 `docs/reviewed/test-catalog.md` §0.2，不在本 README 维护第二张执行状态表。

## 4. 用例规范

Given/When/Then 是可证伪观察。细合同的 `When I run` 通过 test-only embedded application seam；显式 `--server $server` 则走正式 `cli.Run → Client → HTTP`。接入与替换部署的正式配置恢复由 `TestDeploymentSurvivesInstanceReplacement` / `TestDeploymentMissingDurableStateFailsClosed` 提供证据；同一消费者任务也必须经 Server，不允许以 owner 代替发现或 pin。

**必须：**

1. 一条场景独占一种失败风险。construct = 进入可复用前态；probe = 从该前态出发验证另一种风险（授权、冻结入口、隔离），可产生仅属于本用例的后态。
2. 变化之后必须观测。回执不是终点：再用 `status` / `show` / `grant list` / `schema list` / `read` / `projection describe` / SEARCH 读后态。
3. `Then the command succeeds` 只表示退出码 0 且 stdout 是 JSON，**不是后态**。禁止单独使用；`TestSceneFeaturesPinObservedState` 会红。
4. 钉稳定字段：Catalog/仓/principal/action/夹具 Canonical/空数组/`archived`/`retired`。HEAD / `newCommit` 用 `nonempty`。产品库存必须 `home`、`namespace` 为 `absent`。
5. 写入 construct 必须出现 `When I run kc writer commit|writer put`，并钉写入回执。System Schema 用 READ/list，不用 Writer。
6. construct 只断言这一步允许改的七列（Snapshot / Catalog / pin / Canonical / ControlState / 投影）。失败路径钉错误码，不要再 dump 一遍成功态。
7. 表行必须以 `|` 开头和结尾，两列：路径、期望值。

```gherkin
Given deployment fixture
When I run `kc show`
Then the output has:
  | catalogId  | kr://scene/catalog |
  | datasets | [] |
```

**DSL（`cli/scene_feature_test.go` 解析）：**

| 步骤 | 作用 |
|---|---|
| `Given material <id>` | 已退出树脊。写入必须是 `kc writer`；执行器仍解析该步骤，但 `TestSceneWriteSpineUsesPublicWriter` 禁止 construct 再用它 |
| `Given deployment fixture` | 构建细合同的空 Catalog 与 System 信任根前态；不是用户命令，不创建业务 Snapshot |
| `Given existing repository <id>` | 墙外测试夹具提供既有 Snapshot binding，尚未入 Catalog；接入仍由公开 attach 完成 |
| `Given configured catalog <id>` / `Given bootstrap principal <id>` | 多 Catalog / 首次部署授权的夹具前态；真实初始化和恢复另走正式配置入口 |
| `Given local HTTP server` | 测试认证器上的 typed Server；后续带 `--server $server` 的命令走正式 Run |
| `When I run \`kc ...\`` | 公开 argv 的应用合同；带 `--server` 时是正式 Client。`$materials` → 本节点 `_materials/`，`$home` → 本趟 home，`$server` → Given local HTTP，`$last.field` / `$previewId` → 上次成功 CLI JSON |
| `When HTTP METHOD /path [as principal]` | 打刚才起的 HTTP |
| `Then the output has` / `includes` | JSON 路径等于 / 数组包含 |
| `Then error CODE` | 协议错误码 |
| `Then 1 hit <object>` | CLI SEARCH 按 `objectId` 定位；HTTP SEARCH 命中 |
| `Then 1 hit <object> with body stripped` / `with full canonical` | HTTP SEARCH 投递链（无 `knowledge.read` 时屏蔽正文） |
| `Then 0 hits` | 无命中 |
| `Then READ body is full canonical` | READ 正文 |
| `Then whoami is <principal>` | 身份绑定 |

匹配器：`[]` 空数组、`absent` 键不存在、`nonempty`、`{}` 空对象、`foo.bar` 点号路径、`foo[].id` 数组任一元素。数字按 `fmt.Sprint`（JSON `1` → `"1"`）。

`"""` 里：普通段落是给人看的 brief；`Agent as <principal> (search-only|search+read)` 是 Agent 任务，Go 不拿它当 Oracle。

## 5. 需要守住的不变量

场景树自己用合同钉死（删掉会红）：

| 合同 | 禁止观察 |
|---|---|
| `TestSceneCatalogTreeFollowsLayersAndRoles` | 目录父与节点对不上；feature 节点无 construct；场景进 `cli/testdata` |
| `TestSceneFeaturesPinObservedState` | 裸 `command succeeds`；`When I run` / HTTP 后无观测；material 不回读 |
| `TestSceneSystemSchemaMaterialsMatchEmbed` | 旅程夹具与 `knowledge/system/schemas` 漂移 |
| `TestSceneCatalogDoesNotRegisterMaterials` | 场景根出现 `catalog.yaml`；construct 用 `Given material` 代替 Writer |
| `TestSceneCatalogCoversPublicProductSurfaces` | 公开命令没有 capability 挂载状态 |
| `TestSceneFeaturesCoverPublicCLI` | 公开命令既无场景 `When I run`，也无该 capability 对应状态引用的具名正式 Go 测试调用 |
| `TestSceneFeaturesCoverHelpShortestPaths` | help consume/write/compose 最短路径缺 `When I run` |
| `TestSceneCatalogCoversPermissionActions` | 接口表动作没有具体验证入口 |
| `TestSceneBundlesDeclareExistingEntryState` | bundle 没有可构建的既有前态，或所有任务都强制从首次初始化开始 |
| `TestSceneConsumerTaskKeepsOneAuthenticatedPrincipal` | 消费任务中途换主体或绕过正式 Server |
| `TestSceneJourneysRetireLocalAndSeparateRegister` | 正向旅程调用退役的 local / 独立 register |
| `TestSceneViews` | 视图丢失节点、关系或验证证据；隐藏前置后伪造依赖；节点标签误选全部 probe；产品引用失效或条目既无证据关联也无明确缺口 |
| `TestSceneRepositoryVerificationCasesStayOnAttachedState` | 样板段把一次性验证重新变成状态，或搬移时丢失权限与 Collector 证据 |
| `TestSceneProbesIsolateAuthorityAndGrants` / `TestSceneProbesIsolateHTTPAndClientSessions` | probe 顺序、登录、撤权或远端写入污染另一 probe 或后继状态 |
| `TestSceneMutatingGrantProbesAreIsolated` | 真实场景的发权用例调到最前或重复执行后，原先的拒绝与其它用例不再成立 |
| `TestSceneProbeReplayIsolatesUncloneableAuthority` / `TestSceneProjectionProbesRestoreSharedBaseline` | 不可克隆 authority 或派生投影没有在用例前恢复构建基线 |
| `TestSceneProbesRestoreAbsentProjection` / `TestSceneIndexedWorldDoesNotLeakIntoUnindexedWorld` | 用例临时建出的索引使其它用例或独立状态失去“尚未准备投影”的前态 |

协议不变量不在本 README 复述。本树的旅程必须能作为下列证据的可读过程，但不能改写它们：

- `AUTH-01` 搜宽读严、无读权屏蔽正文、不标 `partial`
- `AUTH-02` Dataset `file.read` 不放行仓 `knowledge.*`
- `AUTH-03` 交付链不改 ID/Address
- System Repository 可读、对业务 Writer 不可写（U1）
- Domain Schema 在目标仓版本化（U2）；实例符合 Schema（U3）

失败时七列主状态不变。`--as` / hook / gate 是 facade。授权分叉互不隐含。

## 6. 功能面

树上看哪段状态，不另编功能点 id。Help 三主题只是分组。没有名为 `connector-registered` 的状态：runtime / 凭证在墙外。

功能树与关注点子图均由上面的 `tree.py` 命令实时生成，不在 README 再维护一份全树清单。

全树按同一规则整理。例如 `repository-attached`：库存身份可见但正文拒绝的发权与验证过程在
`_probes/probe-inventory-without-body.feature`；仓已认证默认可读与墙外 Collector Preview
分别挂在本节点的 Go evidence 中。验证结果不再建立后继状态，原断言和具名证据保留。Schema 浏览、部署恢复、文件计划等独立 Go 用例也只挂在相关宿主的 evidence 中。

## 7. 覆盖如何评估

具名 Go Oracle 可以覆盖正式部署场景：capability 必须映射到相关宿主及具体验证，其 go-test process 必须指向具体 Test 函数；正式 CLI 命令静态守卫只认函数内直接 `cli.Run` 或 `kcRemote` 的字面命令前缀，不将文件引用、动态拼接或 embedded 调用算作正式命令证据。运行成功与失败边界仍由实际测试及命令覆盖报告证明。托管建仓与部署恢复用例挂载在初始化前态，使用具名 Go evidence。

分开记录命令触达、协议后态、完整任务、正式 Server 传输和真实依赖证据。命令出现一次、某行有 Then、目录是叶子，都不能代替任务覆盖。

时间局部任务至少声明既有状态和本次目的，并证明同一主体可以依次取得所需材料、固定版本、完成操作和验证结果。部署接入、首次发布、既有知识修改、固定 pin 后的上游变化、治理失败恢复、撤权后的下一请求分别评估；不能把 operator 的 setup 插到消费者任务中替他取得权限。`dataset-cli-discovers-searches-reads.feature` 守卫要求所有消费命令带同一主体及 Server。

重部署验收必须保存 Catalog Snapshot 权威、知识 Snapshot、授权/Gate、Writer receipt、ControlState 和原始证据，只替换进程工作缓存。缺耐久状态应失败关闭；清空派生投影可以触发重建。初始化前态上的部署恢复证据引用正式 Run/Server 测试，不把重新 init 的成功当作恢复。
