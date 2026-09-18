# 协议旅程场景

协议用例怎么组织、维护、执行、怎么写断言。覆盖格子（状态 × 操作、已定位/partial/gap）仍以 [`docs/TEST_CATALOG.md`](../../docs/TEST_CATALOG.md) 为准。架构不变量 ID 以 [`docs/ARCHITECTURE_INVARIANTS.md`](../../docs/ARCHITECTURE_INVARIANTS.md) 为准。清河茶铺走查夹具写在 `named-repositories-created` 叶上，不要另起独立数仓黑盒套件。

这不是检索应用的故事包，也不是 `cli/testdata/`。

## 1. 组织

从父状态冻结的 home 副本上只跑本节点 construct。打开一个节点目录就能读懂当前状态、怎么进来、夹具在哪、停在这里探什么。执行器不把 `_results/` 当 Oracle。

| 目录 | 含义 |
|---|---|
| 不以 `_` 开头的子目录 | **分叉**：后继状态。父目录就是前态 |
| `_meta.yaml` | 本节点自描述：角色、layer、挂哪些 probe/Go 证据。不是树的第二份抄本 |
| `_bundles.yaml` | 从本节点起的时间局部任务（可选） |
| `_build/` | 本节点如何从父状态进入。`construct.feature` 必须可执行 |
| `_materials/` | 这一步用的夹具。`kc writer commit --dir $materials/…` 或 `put --file $materials/…`。`$home` 是本趟 home。System Schema 例外 |
| `_probes/` | 停在本节点上的探，不是新分叉 |
| `_results/` | 本节点上次验证的 `latest.json`。gitignore，不是分叉或 Oracle；不得跨节点拼成同次全部通过 |

公开命令闭集由测试钉在 construct / probe（或 `_meta.yaml` 点名的 go-test）上，不另写一份覆盖清单：

- 每条 `cliSurface` 命令必须在某个状态的 `When I run` 里出现，或该状态 `_meta.yaml` 点名的正式 go-test
- `serve` / `kcfs` 不在 `cliSurface`，由 `TestSceneCatalogCoversPublicProductSurfaces` 另钉
- 不要在场景目录放名为 `catalog.yaml` 的文件（那是产品 Catalog 登记表）
- 不要在场景根放 `sources.yaml`、`checklist.md`，也不要再手写一棵「覆盖对照」树。节点标签留在 `_meta.yaml`

整体视图（给人 / Agent）：`python3 .data/scenes/tree.py` 或 `--json`。从目录抽出，不手写第二份树。

具名 bundle 写在入口节点的 `_bundles.yaml`：声明从该既有状态开始的任务。执行器复用祖先夹具，用户步骤无需从初始化重复。

Catalog 固定 `kr://scene/catalog`。业务知识仓 `kr://scene/knowledge`、关系仓 `kr://scene/graph` 只给后续 `attach` 和「已配置、尚未登记」的探用，**不**在 `catalog-initialized` 进入登记表。`Given deployment fixture` 不创建业务 Snapshot。知识集 `scene-set` 发布时只纳入指定实体/关系路径，不是整仓别名。树自包含，不读数仓目录。

**树脊（接入方）**：`catalog-initialized` → `system-schema-published` → `repository-attached`（服务验证既有 authority 并原子登记）→ `domain-schema-published` → `semantic-knowledge-constructed` →（声明式索引）`projection-synced`。`repository-attached` 是接入完成；后续 commit、put 是知识发布。此脊验证外部既有 Snapshot 接入；部署启动和 attach 不隐式创建它。并列的 `managed-repository-created` 使用正式配置 Go 旅程，验证已授权接入方显式 create 平台仓、立即发布以及重部署后继续操作，目标仓无需静态配置。

树父是**构建前置**，不是权限蕴含：`knowledge.read` 挂在 SEARCH 下是叙事顺序，搜宽读严仍成立。

## 2. 维护

新增或改状态时：

1. 目录嵌套就是前态（只有一个父）。每个状态目录要有 `_meta.yaml`。公开命令必须落在某个 construct/probe 或点名的 go-test 里（测试钉闭集，不另抄清单）。
2. `runner: scene` 必须有 `_build/construct.feature`。construct 只做进入该状态所需的那一步，再观测后态。
3. 夹具放在**写入它的那个节点**的 `_materials/`，不进 Catalog 登记表，不在场景根放 `materials:` 清单。
4. Domain Schema 草稿放在 `domain-schema-published/_materials/drafts/`，construct 跑 `kc writer commit --repo --dir $materials/drafts`。实例与 note 用 `kc writer put --file $materials/…`，再 READ/browse 回读。不要用 `Given material` 代替这些公开命令。Gherkin 不出现 ChangeSet 文件。提交前对照用 `kc diff --repo --dir`，不是 construct 必经步骤。
5. System Schema：跟踪源是 `knowledge/system/schemas/`（`go:embed` 信任根）。`system-schema-published/_materials/` 是旅程可见副本，**禁止** Writer PUT 到 `kr://kc/system`。改协议 Schema 先改跟踪源，再让副本与 `TestSceneSystemSchemaMaterialsMatchEmbed` 对齐。
6. 不要引用已删除的数仓黑盒套件。清河茶铺表/作业/语义/SQL 只写在走查叶 `_materials/`。接入方 Resource Access / Collector / Observer 的 Docker 与代码放在写入 origin 的节点 `_materials/accessor/`。
7. 不要在仓库根加 `tests/scenarios/` 或把协议场景拷进 `cli/testdata/scenes`。
8. 一条 feature 一个 Scenario。construct 与每个 probe 文件各一种失败风险。会改授权状态的探必须排在同节点其它探之后（`_probes/*.feature` 文件名排序即执行序）。
9. 观测走 `kc` / typed HTTP，不打开内部状态，不把 `_results/` 当断言。配置、权威数据、耐久控制状态、证据与可丢缓存分别维护。
10. `file.read`×Dataset / `dataset.resolve` / `retire` 的探走 scene 执行器（`--dataset` SEARCH/READ 是消费旅程，不是为绿而抄命令）。`observation-refreshed` 仍 go-test。成员仓解析的细 Oracle 仍可并列 go-test。

## 3. 执行

执行器 DFS `.data/scenes/`：凡有 `_build/construct.feature` 的节点，**复用父节点 construct 之后冻结的 home 副本**（测试 TempDir，gitignore 的 `_results/` 只记本次是否绿，不当断言）。在副本上只跑本节点 construct，再跑 `runner: scene` 的 `_probes`。父 home 尚未冻结时（单节点 `-run`），才从祖先 construct 链重建并冻结。

`Given existing repository` 由执行器接到 lakeFS Snapshot adapter。Gherkin 不写介质名。`make test` 的 `TestProductScenes` 用进程内协议忠实假服务，父 home 副本为每个 lakefs stamp 分叉独立物理仓。第一阶段部署拓扑上的同一棵树走 `TestLiveLakeFSSceneDFS`：真 Graveler + MinIO + PostgreSQL + OpenSearch（`make deploy-local-scenes`）。真 Graveler 不能按 commit id 分叉，所以 live DFS 每个节点从根重放，不克隆父 home。Catalog 组件夹具仍是 gitdir，不是生产 OpenSnapshotRegistry。

两条写入脊不要并成一条：`knowledge-published` 挂在 `repository-attached` 上（普通 Canonical）；`semantic-knowledge-constructed` 挂在 Domain Schema 发表之后（语义实例）。目录嵌套与 `depends_on` 必须同时改。

```bash
export PATH="$HOME/.local/go/bin:$PATH"
go test ./cli -run 'TestSceneCatalog|TestSceneFeaturesPinObservedState|TestSceneFeaturesCoverPublicCLI|TestSceneFeaturesCoverHelpShortestPaths|TestSceneConsumeJourneyIsOneFeature|TestSceneWriteSpine|TestSceneExecutorDiscovers|TestSceneGoTestFeatures|TestSceneExecutorReusesParentConstructHome'
go test ./cli -run 'TestProductScenes'                                   # 不构建检索投影、不跑动态 State
KC_TEST_OPENSEARCH_URL=http://127.0.0.1:19200 \
  go test ./cli -run 'TestMetricPermissionScenes'                        # 祖先含 projection-synced，或 construct 含 projection sync
go test ./cli -run 'TestProductScenes/system-schema-published$'           # 单节点
make deploy-local-scenes                                                 # 同一棵树，真 Graveler + OpenSearch
python3 .data/scenes/goto.py sales-dataset-defined                       # 把走查 Server 重放到 Dataset 叶
make deploy-local-goto NODE=sales-dataset-defined
```

- `TestProductScenes` 跳过需要索引或动态 State 的节点。
- `TestMetricPermissionScenes` 需要 OpenSearch（`make test` 会起一次性实例；手跑要自己设 URL）。
- `TestLiveLakeFSSceneDFS` 在 `KC_SCENE_LIVE_LAKEFS_URL` 上 DFS 全树；部署拓扑由 `make deploy-local-scenes` 注入 lakeFS / OpenSearch。`observation-refreshed` 仍要独立 State runtime，不进 `TestProductScenes`。
- 走查 Server 用 `python3 .data/scenes/goto.py <id或目录>` 重放该节点祖先 construct。`catalog-initialized` 已由 `deploy-local-up` 初始化，脚本跳过它。`named-repositories-created` 分支需要托管 create 与墙外 compose，不进 `TestProductScenes`。
- `Agent as` 任务块给人 / KC-AGENT-01，**不是**协议 Oracle；协议绿看 Then。
- 局部 `go test` 只用于定位，不能代替 `make test`。

现场 ttyd 敲的是产品 `kc`，不展开 `$materials` / `$home` / `$last.id`。夹具用该节点 `_materials/` 的真实路径；`--id` 从刚才的 JSON 抄。不要 `export KC_AS`：它盖住 login 会话。live `deployment init` 会写入 bootstrap 管理主体，空 allow 的 `catalog-allow-ready` 只存在于 InitHome 夹具。

需要保留可复核的定向证据时，使用
`python3 scripts/validation.py run --scope scene-debug -- go test -json -count=1 -run '<选择式>' ./cli`。
它只运行显式选择，不自动补跑其它场景。正式 testsuite 自带同次运行目录；scene 同时保存
`scenes/<test>/<state>-<execution-id>.json`，包含 run-id、源码指纹、测试名、唯一执行身份、
开始/结束时间和观察步骤。同次 run 重跑相同 Test/node 也保留各次结果，不覆盖前次失败。
节点 `latest.json` 只是方便就近查看的副本；无 run-id 的直接运行文件不具有版本绑定能力。
完整库存由 `make validation-inventory` 读取覆盖表与场景目录生成；结果与 skip/未观测的区别
统一遵循 `docs/TEST_CATALOG.md` §0.2，不在本 README 维护第二张执行状态表。

## 4. 用例规范

Given/When/Then 是可证伪观察。细合同的 `When I run` 通过 test-only embedded application seam；显式 `--server $server` 则走正式 `cli.Run → Client → HTTP`。接入与替换部署的正式配置恢复由 `TestDeploymentSurvivesInstanceReplacement` / `TestDeploymentMissingDurableStateFailsClosed` 提供证据；同一消费者任务也必须经 Server，不允许以 owner 代替发现或 pin。

**必须：**

1. 一条场景独占一种失败风险。construct = 进入该状态；probe = 停在该状态上的另一种风险（授权、冻结入口、隔离）。
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
| `TestSceneCatalogCoversPermissionActions` | 接口表动作没有场景状态 |
| `TestSceneBundlesDeclareExistingEntryState` | bundle 没有可构建的既有前态，或所有任务都强制从首次初始化开始 |
| `TestSceneConsumerTaskKeepsOneAuthenticatedPrincipal` | 消费任务中途换主体或绕过正式 Server |
| `TestSceneJourneysRetireLocalAndSeparateRegister` | 正向旅程调用退役的 local / 独立 register |

协议不变量不在本 README 复述。本树的旅程必须能作为下列证据的可读过程，但不能改写它们：

- `AUTH-01` 搜宽读严、无读权屏蔽正文、不标 `partial`
- `AUTH-02` Dataset `file.read` 不放行仓 `knowledge.*`
- `AUTH-03` 交付链不改 ID/Address
- System Repository 可读、对业务 Writer 不可写（U1）
- Domain Schema 在目标仓版本化（U2）；实例符合 Schema（U3）

失败时七列主状态不变。`--as` / hook / gate 是 facade。授权分叉互不隐含。

## 6. 功能面

树上看哪段状态，不另编功能点 id。Help 三主题只是分组。没有名为 `connector-registered` 的状态：runtime / 凭证在墙外。

```text
.data/scenes/
  tree.py                                       # 从目录抽出整体视图
  catalog-initialized/                          # ① 登记表出生
    _meta.yaml
    _bundles.yaml                               # bootstrap
    _build/construct.feature
    _probes/probe-status.feature
    schema-browsed/
    http-served/
      access-audited/
    absent-product-surfaces/
    catalog-allow-ready/                        # Catalog 授权面（空规则）
      grants-bootstrapped/
      catalog-read-granted/
        grant-revoked/
      catalog-audit-granted/
      catalog-create-granted/
      catalog-declared-private/
    named-repositories-created/                 # 走查：create+attach table-meta、sales-semantic；清河茶铺夹具
      access-runtime-ready/                     # 走查：两仓用例知识 + table-meta 接入方容器
        index-ready/                            # 投影追上；SEARCH 从索引访问
          sales-dataset-defined/                # 叶：两仓 tables/semantic-models/metrics → qinghe-sales
    system-schema-published/                    # 接入方读 System Schema
      _materials/                               # 与 knowledge/system/schemas 对账
      _probes/probe-system-immutable.feature
      repository-attached/                      # 只读验证既有源并原子提交成员登记
        catalog-inventory-visible/
        repository-declared-readable/           # 仓已认证默认可读；go-test
        domain-schema-published/            # kc writer commit --dir
          semantic-knowledge-constructed/
              _materials/metric.gmv.json      # kc writer put
              projection-synced/
                knowledge-search-granted/
                  knowledge-read-granted/
              knowledge-set-defined/            # 两仓：metric/gmv + rel/defines/gmv
                principals-granted/
                dataset-manage-granted/
        knowledge-published/
          _materials/note.hello.json          # kc writer put
          dataset-defined/
            proposal-opened/                  # create
              proposal-previewed/             # preview
                proposal-validated/           # validate
                  validation-recorded/        # record
                    proposal-merged/          # merge
```

## 7. 覆盖如何评估

具名 Go Oracle 可以覆盖正式部署场景：capability 必须映射到该状态，其 go-test process 必须指向具体 Test 函数；静态守卫只认函数内直接 `cli.Run` 或 `kcRemote` 的字面命令前缀，不将文件引用、动态拼接或 embedded 调用算作正式命令证据。运行成功与失败边界仍由实际测试及命令覆盖报告证明。`managed-repository-created` 不编造 feature 或可执行 bundle。

分开记录命令触达、协议后态、完整任务、正式 Server 传输和真实依赖证据。命令出现一次、某行有 Then、目录是叶子，都不能代替任务覆盖。

时间局部任务至少声明既有状态和本次目的，并证明同一主体可以依次取得所需材料、固定版本、完成操作和验证结果。部署接入、首次发布、既有知识修改、固定 pin 后的上游变化、治理失败恢复、撤权后的下一请求分别评估；不能把 operator 的 setup 插到消费者任务中替他取得权限。`probe-dataset-cli.feature` 守卫要求所有消费命令带同一主体及 Server。

重部署验收必须保存 Catalog Snapshot 权威、知识 Snapshot、授权/Gate、Writer receipt、ControlState 和原始证据，只替换进程工作缓存。缺耐久状态应失败关闭；清空派生投影可以触发重建。对应状态 `deployment-restored` 引用正式 Run/Server 测试，不把重新 init 的成功当作恢复。
