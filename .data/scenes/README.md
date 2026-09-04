# 协议旅程场景

协议用例怎么组织、维护、执行、怎么写断言。覆盖格子（状态 × 操作、ok/partial/gap）仍以 [`docs/TEST_CATALOG.md`](../../docs/TEST_CATALOG.md) 为准。架构不变量 ID 以 [`docs/ARCHITECTURE_INVARIANTS.md`](../../docs/ARCHITECTURE_INVARIANTS.md) 为准。数仓黑盒是另一棵夹具树，不要和本树混写。

这不是检索应用的故事包，也不是 `cli/testdata/`。

## 1. 组织

从空 home 一步步构建。打开一个节点目录就能读懂当前状态、怎么进来、夹具在哪、停在这里探什么。

| 目录 | 含义 |
|---|---|
| 不以 `_` 开头的子目录 | **分叉**：后继状态。父目录 = `catalog.yaml` 的 `depends_on` |
| `_build/` | 本节点如何从父状态进入。`construct.feature` 必须可执行 |
| `_materials/` | 这一步用的夹具。`kc pack --dir $materials/…` 或 `put --file $materials/…`。`$home` 是本趟 home（ChangeSet `--out`）。System Schema 例外 |
| `_probes/` | 停在本节点上的探，不是新分叉 |
| `_results/` | 上次验证的 `latest.json`。gitignore，不是分叉，也不是 Oracle |

`catalog.yaml` 登记三张分母，不是执行顺序：

- `capabilities`：公开 CLI + `serve` + `kcfs`
- `actions`：`PERMISSIONS.md` 接口表（互不隐含）
- `features`：可提供的功能点。`runner: scene` 必须有 construct；`go-test` 沿用已有 Oracle；**只有** `identity.taihu-live` 需要真人（`KC_LIVE_TAIHU=1`）

具名 `bundles` 是给人 / Agent 的旅程摘录，**不是**执行分母。

仓坐标固定 `kr://scene/catalog` / `kr://scene/knowledge`，知识集 `scene-set`。树自包含，不读数仓目录。

**树脊（接入方）**：`catalog-initialized` → `system-schema-published` → `repository-attached` → `drafts-ingested` → `domain-schema-published` → `semantic-knowledge-constructed` →（声明式索引）`projection-synced`。`repository-attached` 是打开空知识仓（本机 attach + Catalog 登记），不是接入完成；接入过程必须再走公开 `kc pack` / `commit` / `put`。

树父是**构建前置**，不是权限蕴含：`knowledge.read` 挂在 SEARCH 下是叙事顺序，搜宽读严仍成立。

## 2. 维护

新增或改状态时：

1. 目录嵌套必须等于 `depends_on`（只有一个父）。改树要同时改目录和 `catalog.yaml`，并升高 `version`。
2. `runner: scene` 必须有 `_build/construct.feature`。construct 只做进入该状态所需的那一步，再观测后态。
3. 夹具放在**写入它的那个节点**的 `_materials/`，不进 Catalog 登记表，不在 `catalog.yaml` 加 `materials:`。
4. Domain Schema 草稿放在 `drafts-ingested/_materials/drafts/`，construct 跑 `kc pack`（钉未发表）。`domain-schema-published` 跑 `kc writer commit --changeset $home/…`。实例与 note 用 `kc writer put --file $materials/…`，再 READ/browse 回读。不要用 `Given material` 代替这些公开命令。
5. System Schema：跟踪源是 `knowledge/system/schemas/`（`go:embed` 信任根）。`system-schema-published/_materials/` 是旅程可见副本，**禁止** Writer PUT 到 `kr://kc/system`。改协议 Schema 先改跟踪源，再让副本与 `TestSceneSystemSchemaMaterialsMatchEmbed` 对齐。
6. 不要把数仓表名、Hive GRANT、compose 或数仓夹具目录路径写进本树。
7. 不要在仓库根加 `tests/scenarios/` 或把协议场景拷进 `cli/testdata/scenes`。
8. 一条 feature 一个 Scenario。construct 与每个 probe 文件各一种失败风险。
9. 观测走公开 `kc` / local HTTP，不打开 `.kc`，不把 `_results/` 当断言。
10. `workspace.consume` / `resolve` / `retire` 打开成员仓解析的探仍走既有 Go 测试（`runner: go-test`），不要为了绿而在 scene 执行器里加 `--workspace` SEARCH/READ。

## 3. 执行

执行器 DFS `.data/scenes/`：凡有 `_build/construct.feature` 的节点，从空 home 重建**可 construct 祖先链**，停在该节点；`runner: scene` 的 `_probes` 一并跑，写入 `_results/latest.json`。

```bash
export PATH="$HOME/.local/go/bin:$PATH"
# 随 make test / scripts/testsuite.sh 进入 ./cli
go test ./cli -run 'TestSceneCatalog|TestSceneFeaturesPinObservedState'   # 树合同 + feature 观测合同
go test ./cli -run 'TestProductScenes'                                   # 不构建检索投影、不跑动态 State
KC_TEST_OPENSEARCH_URL=http://127.0.0.1:19200 \
  go test ./cli -run 'TestMetricPermissionScenes'                        # 祖先含 projection-synced，或 construct 含 projection sync
go test ./cli -run 'TestProductScenes/system-schema-published$'           # 单节点
```

- `TestProductScenes` 跳过需要索引或动态 State 的节点。
- `TestMetricPermissionScenes` 需要 OpenSearch（`make test` 会起一次性实例；手跑要自己设 URL）。
- `observation-refreshed` / `projection notice` 走既有 go-test，不进上述两个套件。
- `Agent as` 任务块给人 / KC-AGENT-01，**不是**协议 Oracle；协议绿看 Then。
- 局部 `go test` 只用于定位，不能代替 `make test`。

## 4. 用例规范

Given/When/Then 是可证伪观察。人读和 Go 跑同一份 feature。

**必须：**

1. 一条场景独占一种失败风险。construct = 进入该状态；probe = 停在该状态上的另一种风险（授权、冻结入口、隔离）。
2. 变化之后必须观测。回执不是终点：再用 `status` / `show` / `grant list` / `schema list` / `knowledge read` / `projection describe` / SEARCH 读后态。
3. `Then the command succeeds` 只表示退出码 0 且 stdout 是 JSON，**不是后态**。禁止单独使用；`TestSceneFeaturesPinObservedState` 会红。
4. 钉稳定字段：Catalog/仓/principal/action/夹具 Canonical/空数组/`archived`/`retired`。HEAD / `newCommit` 用 `nonempty`。`local status` 必须 `home`、`namespace` 为 `absent`。
5. 写入 construct 必须出现 `When I run kc pack|commit|put`，并钉写入回执或「未发表」后态。System Schema 用 READ/browse，不用 Writer。
6. construct 只断言这一步允许改的七列（Snapshot / Catalog / pin / Canonical / ControlState / 投影）。失败路径钉错误码，不要再 dump 一遍成功态。
7. 表行必须以 `|` 开头和结尾，两列：路径、期望值。

```gherkin
When I run `kc local init --catalog kr://scene/catalog`
Then the output has:
  | catalog             | kr://scene/catalog |
  | system.repositoryId | kr://kc/system |
When I run `kc catalog show`
Then the output has:
  | catalogId  | kr://scene/catalog |
  | workspaces | [] |
```

**DSL（`cli/scene_feature_test.go` 解析）：**

| 步骤 | 作用 |
|---|---|
| `Given material <id>` | 已退出树脊。写入必须是 `kc writer`；执行器仍解析该步骤，但 `TestSceneWriteSpineUsesPublicWriter` 禁止 construct 再用它 |
| `Given local HTTP server` | 进程内 local serve |
| `When I run \`kc ...\`` | 公开 CLI（`--home` 由执行器注入）。`$materials` → 本节点 `_materials/`，`$home` → 本趟 home |
| `When HTTP METHOD /path [as principal]` | 打刚才起的 HTTP |
| `Then the output has` / `includes` | JSON 路径等于 / 数组包含 |
| `Then error CODE` | 协议错误码 |
| `Then 1 hit <object>` / `with body stripped` / `with full canonical` | SEARCH 命中 |
| `Then 0 hits` | 无命中 |
| `Then READ body is full canonical` | READ 正文 |
| `Then whoami is <principal>` | 身份绑定 |

匹配器：`[]` 空数组、`absent` 键不存在、`nonempty`、`{}` 空对象、`foo.bar` 点号路径、`foo[].id` 数组任一元素。数字按 `fmt.Sprint`（JSON `1` → `"1"`）。

`"""` 里：普通段落是给人看的 brief；`Agent as <principal> (search-only|search+read)` 是 Agent 任务，Go 不拿它当 Oracle。

## 5. 需要守住的不变量

场景树自己用合同钉死（删掉会红）：

| 合同 | 禁止观察 |
|---|---|
| `TestSceneCatalogTreeFollowsLayersAndRoles` | 目录父 ≠ `depends_on`；feature 节点无 construct；场景进 `cli/testdata` |
| `TestSceneFeaturesPinObservedState` | 裸 `command succeeds`；`When I run` / HTTP 后无观测；material 不回读 |
| `TestSceneSystemSchemaMaterialsMatchEmbed` | 旅程夹具与 `knowledge/system/schemas` 漂移 |
| `TestSceneCatalogDoesNotRegisterMaterials` | `catalog.yaml` 出现 `materials:` |
| `TestSceneCatalogCoversPublicProductSurfaces` | 公开命令没有 capability 挂载状态 |
| `TestSceneCatalogCoversPermissionActions` | 接口表动作没有场景状态 |

协议不变量不在本 README 复述。本树的旅程必须能作为下列证据的可读过程，但不能改写它们：

- `AUTH-01` 搜宽读严、无读权屏蔽正文、不标 `partial`
- `AUTH-02` `workspace.consume` 不放行 `knowledge.*`
- `AUTH-03` 交付链不改 ID/Address
- System Repository 可读、对业务 Writer 不可写（U1）
- Domain Schema 在目标仓版本化（U2）；实例符合 Schema（U3）

失败时七列主状态不变。`--as` / hook / gate 是 facade。授权分叉互不隐含。

## 6. 功能点挂载

| 组 | 功能点 | 场景 | 用例 |
|---|---|---|---|
| 宿主 | `local.init` / `system.schema` / `local.repository` / `local.store` | 已 init / System Schema / 已挂仓 | scene 或 go-test |
| 写入 | `writer.ingest` / `writer.commit` / `knowledge.publish` / `connector.preview` | 草稿预览 / Schema COMMIT / Canonical PUT / Preview | scene 或 go-test |
| Schema | `schema.publish` / `schema.browse-mechanics` / `knowledge.schema.read` | Domain Schema / 分页 / schema.read | scene、go-test |
| 发现 | `catalog.read` / `catalog.source-profile` / 知识集 define·resolve·consume·retire·federated / `catalog.archive` | 库存与源说明、组合与生命周期 | scene 或 go-test |
| 授权 | local 身份 / Taihu live / 按人配权 / 撤权 / permissions Aspect ≠ 闸门 | http / principals / revoke | scene、search 或 live |
| 索引 | `index.declarative` / `index.dynamic` | Snapshot 投影 / Binding 观察投影 | search 或 go-test |
| 消费 | SEARCH / READ / 溯源 / relations / 交付屏蔽 / rerank | search-granted 等 | search 或 go-test |
| 句柄 | Binding 进仓 / `resource.access` / 观察刷新 | handle / observation | go-test |
| 文件 | File Gateway 计划 / `kcfs` | file-view / mounted | go-test |
| 治理 | 提案 / 合并 / hook / gate | proposal-* | go-test |
| 运维 | access/trace/hitmap | access-audited | go-test |
| 冻结 | MCP、LIST、checkout、connector-run、APPEND | absent-product-surfaces | scene |

Help 三主题只是分组。没有名为 `connector-registered` 的状态：runtime / 凭证在墙外。

```text
.data/scenes/
  catalog.yaml
  catalog-initialized/                          # ① 登记表出生
    _build/construct.feature
    _probes/probe-status.feature
    schema-browsed/
    http-served/
      access-audited/
    absent-product-surfaces/
    system-schema-published/                    # 接入方读 System Schema
      _materials/                               # 与 knowledge/system/schemas 对账
      _probes/probe-system-immutable.feature
      repository-attached/                      # 打开空知识仓
        drafts-ingested/                        # kc pack，未发表
          _materials/drafts/schema.metric.definition.yaml
          domain-schema-published/              # kc writer commit
            semantic-knowledge-constructed/
              _materials/metric.gmv.json        # kc writer put
              projection-synced/
                knowledge-search-granted/
                  knowledge-read-granted/
              knowledge-set-defined/
                principals-granted/
        knowledge-published/
          _materials/note.hello.json            # kc writer put
```
