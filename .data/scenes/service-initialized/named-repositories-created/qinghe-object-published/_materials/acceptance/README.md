# 插件重构验收夹具

对象模型与插件重构完成后的最终验收夹具。用例规格在本节点 `_specs/accept-*.yaml`（见下文“用例树”），设计依据是
[发现](../../../../../../../doc/design/discovery.md)、[LiveAccess](../../../../../../../doc/design/liveaccess.md)
与[插件](../../../../../../../doc/design/plugins.md)。夹具按目标写法编写，重构完成前当前代码可能拒绝或误读它们，
这正是规格要证明的差距。本目录不被本节点 construct 写入，接入可执行用例时由各 probe 显式写入。

## 世界

在清河两仓（`table-meta`、`sales-semantic`）已发布的前态上，再加两个仓：

取值描述 `ops/warehouse-stats` 声明 `identity: shared`（行数与调用者无关，可以进索引）；另两份未写，按默认
`per-caller`，只能读取，不能作为索引输入。

| 仓 | 谁维护 | 内容 | 夹具 |
|---|---|---|---|
| `table-meta`（已有） | 数仓团队 | 新增 `runtime` 方面（成员级外部取值）与 `live-stats` 方面（方面级外部取值）及其 Schema | `table-meta/` |
| `ops-shared`（probe 创建） | 平台团队 | 三份取值描述：`ops/warehouse-stats`、`ops/pipeline-monitor` 指向可信源，`ops/untrusted-monitor` 指向不可信源 | `ops-shared/` |
| `sales-analysis`（probe 创建） | 分析组，即 Dataset 维护者 | 索引声明 `index/sales-search` 的 `index` 方面，四个成员 | `sales-analysis/` |

Dataset `sales-analysis` 选入 `table-meta` 的 `tables`、`sales-semantic` 的 `metrics` 和 `sales-analysis` 的
`indexes`。示意（命令形状以重构后的 `kc dataset define --help` 为准）：

```bash
kc dataset define --dataset sales-analysis --revision 1 \
  --source table-meta=refs/heads/main@tables@tables \
  --source sales-semantic=refs/heads/main@metrics@metrics \
  --source sales-analysis=refs/heads/main@indexes@indexes
```

对照用的 Dataset `sales-plain` 选入同样的 `tables` 与 `metrics`，不选 `indexes`。

`table-meta/` 下的文件是仓内相对路径，在已有内容上逐个 `kc writer put`，不要用整目录 commit 覆盖清河前态；
`ops-shared/`、`sales-analysis/` 是新仓的完整内容，可以 `kc writer commit --dir`。

## 外部取值单元

| 单元 | 值来源 | 测试源默认值 |
|---|---|---|
| `table/shop.orders#/runtime/row-count` | `ops/warehouse-stats` | `value: 6` |
| `table/shop.orders#/runtime/freshness-lag` | `ops/pipeline-monitor` | `value: 120` |
| `table/shop.orders#/runtime/retention-days` | 仓内静态值 | `value: 365` |
| `table/shop.{orders,customers,products}#/live-stats` | `ops/warehouse-stats`（方面级） | `rowCount` 为 6、4、4 |

## 索引声明成员

| 成员 | 输入 | 构建 | 用途 |
|---|---|---|---|
| `table-name` | `table.properties` 的 `name`、`qualifiedName` | text，standard | 全文 |
| `metric-text` | `metric.properties` 的 `name` 加 `metric.definition` 的 `expression`（跨方面拼接，缺失跳过） | text，模板拼接 | 全文 |
| `table-row-count` | `table.live-stats` 的 `rowCount`（外部值进索引） | number | 过滤、排序 |
| `metric-unit` | `metric.definition` 的 `unit` | keyword，lowercase | 过滤 |

`schema/core/index-declaration/v1` 是重构后系统仓发布的索引声明 Schema；`declarative-index/opensearch@2026-10` 是测试部署
给 OpenSearch 实例配的能力清单名，部署把它对应到唯一实例。

## 变体

`variants/` 下的文件不属于默认世界，由单个用例从前态独立写入：

| 文件 | 用途 | 预期 |
|---|---|---|
| `index/metric-semantic.yaml` | 向量成员，模型 `embed/test-hash@1` | 测试部署为实例登记这个确定性模型时可构建；未登记时该成员标为不支持 |
| `index/table-name-boost.yaml` | 同一成员只改打分权重 | 不触发重建 |
| `index/table-name-unsupported-analyzer.yaml` | 清单外的分词器 | Writer 接受；试运行与准备都把该成员标为不支持，其他成员照常 |
| `index/unmapped-catalog.yaml` | 清单名没有对应实例 | 该成员没有能力，其他成员照常 |
| `index/catalog-version-ahead.yaml` | 清单版本比实例新 | 标为不支持，不按实例版本代为构建 |
| `index/table-row-count-duplicate.yaml` | 同一字段两条过滤成员 | 等值过滤报有歧义 |
| `index/metric-text-renamed.yaml` | 换成员键（或与原成员并存比较） | 旧键计数保留，新键从零开始 |
| `index/metric-text-missing-exclude.yaml` | 缺值处理改为 exclude | 缺 definition 的指标不经该成员命中 |
| `index/columns-array.yaml` | 数组字段平铺 | 可按主键列引用过滤 |
| `business-index-aspect/*` | 业务自己叫 `index` 的方面及其 Schema | 普通内容，不被声明式索引认领 |
| `protocol-v2/*` | 取值描述的协议没有对应实例 | Writer 接受；该单元报没有取值能力 |
| `stream/*` | Stream 取值描述及引用它的成员 | 普通读取逐单元拒绝，不外呼 |
| `shared-descriptor/orders-error-rate.yaml` | 第二个成员引用同一取值描述 | 两次调用，失败互不连带 |
| `dataset/follow-default.yaml`、`dataset/follow-manual-semantic.yaml` | Dataset 定义：来源默认跟随；sales-semantic 显式手动 | 跟随来源前进自动发修订；手动来源不动 |
| `dataset/retention-two.yaml` | Dataset 定义：检索保留至少 1 小时且至少 2 个修订，来源全部手动 | 超出保留的修订可读不可搜；退役与续页宽限用例改这两个数 |
| `index/runtime-value-mixed-identity.yaml` | 索引成员覆盖 shared、per-caller 与静态三种单元 | 整条标为不支持并列出 per-caller 单元 |
| `origin-match/*.yaml` | 取值描述换成同 host 不同端口、https、子域名 | 都不带凭证 |
| `declaration-split/*.yaml` | 跨仓配套改动的两半：Schema 改字段名、索引成员跟上 | 只到一半时准备不接受，到齐后自动通过 |
| `deployment/dataset-quota.yaml` | Dataset 配额与自动发布最小间隔 | 超额不接受；间隔内的前进合并 |
| `deployment/lakefs-gc-unsafe.yaml` | 不承认提交保留的 lakeFS 回收规则 | 部署拒绝启动 |
| `deployment/*.yaml` | 部署配置片段：同一清单名两个实例、按仓选择插件 | 部署拒绝启动 |
| `untrusted-freshness-lag.yaml` | 改为引用不可信 origin 的取值描述 | 照常外呼但不带调用者凭证 |
| `rejected/*.yaml` | Schema 带 `origin`/`access`；取值来源内联、未知类型、字段级、引用缺失或类型不对；外部单元带正文；方面级声明下写成员；方面与成员双声明；取值描述带凭证、origin 带用户信息、未知 mode；索引输入为空、定位符非法、目标不存在、夹带引擎或原始 mapping/脚本、自身声明外部取值 | Writer 拒绝，HEAD 不变 |

`rejected/runtime-aspect-declared.yaml` 要在默认 `runtime` 成员已存在时写入，才构成双声明。

## 测试源服务

测试部署使用 `scripts/deploy/compose.accept.yaml` 声明 `live-origin` 与 `untrusted-origin` 两个实例；
前者提供 7400 与 7402 端口以及 `api.live-origin` 别名，后者提供 7401。部署生命周期管理这些服务，
`make deploy-local-accept` 只绑定已经运行的服务，不隐式启动、重建或重置测试栈。
`repository-attached/_materials/accessor` 负责产品树的 Resource Access、Collector 与 Observer；
这里的 `live-origin` 只负责验收所需的外部取值、可控故障与调用记录，两个夹具各自保留。

`live-origin/origin.py` 是只用标准库的 `resource-access/v1` 源。按请求里的单元地址
（`binding.address` 的 `objectId`、`aspectName`、`memberKey`）返回 `units.json` 中的值和观察依据，并记录每次调用。

```bash
python3 live-origin/origin.py --listen 127.0.0.1:7400 --name live-origin --generation live-origin-v1
python3 live-origin/test_origin.py          # 自测
docker compose -f live-origin/compose.yaml up -d   # 部署拓扑：live-origin:7400 与 untrusted-origin:7401
```

| 接口 | 作用 |
|---|---|
| `POST /v1/access` | 取值；缺 `X-Resource-Principal` 返回 401，未知单元返回 404 `OBJECT_REF_UNRESOLVED` |
| `GET /health` | 名称与代际 |
| `GET /control/units`、`PUT /control/units/<地址>`、`DELETE /control/units/<地址>` | 查看、改写、删除单元；每次改写使 `sourceRevision` 加一 |
| `GET /control/calls`、`DELETE /control/calls` | 读取、清空调用记录：地址、取值描述、声明摘要、身份头 |
| `POST /control/reset` | 恢复初始单元并清空记录 |

控制接口里的地址要把 `#` 编码成 `%23`，例如 `/control/units/table/shop.orders%23/runtime/freshness-lag`。

单元行为 `behavior`：`ok`（默认）、`null`（合法空值）、`error`（`status`、`errorCode`、`message`）、
`slow`（`delayMs` 后返回）、`invalid`（返回 `value` 原样，用来违反 Schema）、`bad-basis`（依据缺 `bindingGeneration`）。
用 `--generation` 启动不同代际的实例，可以区分“换了实例”。

## 用例树

规格按关注点分文件，每个文件是树的一枝，条目是叶。`python3 .data/scenes/tree.py --family product` 按设计用例
展示同一批规格；这里按重构要做到的事展示。规格只是待验证的目标，不是执行证据。

| 枝 | 文件 | 要做到 | 边界重点 |
|---|---|---|---|
| 插件是解释器 | `accept-01-interpretation.yaml` | 按 Schema 认领声明；没装时声明是普通内容；清单名或协议对应唯一实例 | 业务同名 `index` 方面不被认领；清单名或协议无实例只影响该单元；重复对应、按仓选择让部署拒绝启动 |
| 取值声明写入 | `accept-02-live-declaration.yaml` | 只认单元头的 `descriptorRef`；Schema 只描述结构 | 双向双声明、外部单元带正文、字段级、未知类型、坏引用、取值描述带凭证；维护读取不外呼；历史只记声明 |
| 读取与观察 | `accept-03-live-read.yaml` | 外部值原位出现，观察挂在值旁，失败逐值隔离 | 空值、超时、404、不可达、非法值或依据、缺能力分开；同一取值描述两个成员各自调用；只解释请求范围；Stream 不经普通读取 |
| 身份与凭证 | `accept-04-live-trust.yaml` | 调用者身份逐值透传，凭证只发可信 origin | 不可信 origin 不带凭证；后台准备用服务身份 |
| 版本与固定 | `accept-05-live-versioning.yaml` | 固定版本按原声明取值；改描述改摘要 | HEAD 改 origin、删取值描述、单元改静态都不影响固定读取；摘要只波及引用者；换实例不改仓 |
| 索引声明写入 | `accept-06-index-declaration.yaml` | `index` 方面经 Writer 校验形状与定位符 | Writer 不判断能力（清单外写法可写入）；跨仓引用 Schema；数组输入 |
| 能力清单 | `accept-07-index-capability.yaml` | 按实例公开带版本清单与试运行 | 清单版本不一致不代换；清单升级只核对受影响成员；试运行与 Writer 判断一致 |
| 构建与失效 | `accept-08-index-build.yaml` | 按声明构建，Schema 或声明变化时正确失效或重建 | 跨方面拼接与缺值处理；外部值进索引；歧义拒绝；无适用对象是完整零命中；索引丢失报未就绪 |
| 检索空间 | `accept-09-search-space.yaml` | 只在 Dataset 里搜索，消费权整体生效 | 按仓搜索被拒；仓读权不替代消费权；撤权后续页被拒；未选进的对象不泄漏；首个修订未就绪 |
| 使用记录 | `accept-10-usage.yaml` | 查询、回读、反馈按成员归因，只公开数据 | 他人或无关回读的反馈被拒，重复反馈只计一次；过滤与打分分开计 |
| 旧路径退役 | `accept-11-retirement.yaml` | 删 Bound Schema、按仓选择插件、按仓检索与 Schema 访问声明 | 结构检查，防止旧路径变成隐藏回退；场景树里按旧写法的用例在对应任务里改写或删除 |
| Dataset 修订 | `accept-12-dataset-revisions.yaml` | 来源默认跟随，分支前进自动发修订并准备索引；逻辑上每个修订一个检索空间，按保留策略回收；来源提交有保留；有配额与紧急清除 | 连续提交只追最新；非快进与删分支停止跟随；跨仓配套改动到一半不接受；最小间隔；准备失败保留当前修订；手动来源不动；撤许可停止跟随；保留按时间加数量；任务固定不延长保留；续页宽限；退役读搜都拒；当前修订不能退役；回收不越过保留；超配额不接受；清除覆盖共用索引与证据 |
| 交叉边界 | `accept-13-cross-cutting.yaml` | 外部值与索引、时限、可信范围叠加时不出漏洞 | 只有 shared 外部值进索引，混合身份整条不支持；被外部系统拒绝的人搜不到；命中回读补判；旧修订不刷新外部值；取值描述仓未选入；请求总时限；origin 精确匹配 |

`repository-attached/_specs/liveaccess.yaml` 中 U13～U20 的规格沿用原位置，已改为同一口径（插件由部署按协议对应实例，不按仓选择）。

### 已定的边界

此前列为尚待确认的三项已写入 [LiveAccess](../../../../../../../doc/design/liveaccess.md)，规格在 `accept-13-cross-cutting.yaml`：

- 取值描述所在仓没选入 Dataset 时，按单元引用的版本解析，取值描述不进入消费者可见范围。
- 一次读取的外部取值共用请求剩余时限，用尽后其余单元逐个报取值超时。
- origin 与可信范围按 scheme、host、port 精确匹配，不通配。
