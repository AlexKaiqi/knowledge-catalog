# 零知识验证夹具

验证的问题是：KC 提供的对象与说明，能不能让一个陌生使用者借助通用 Agent 认识环境、理解业务，
并解决业务问题。用例规格在本节点 `_specs/zk-*.yaml`，接入方用例因起点不能含清河业务对象，挂在父节点
`../../_specs/zk-supplier.yaml`。设计依据是[对象](../../../../../../../doc/design/object.md)
U8、U9，[Affordance](../../../../../../../doc/design/affordance.md) U1、U4、U5、U8，
[服务](../../../../../../../doc/design/service.md) U5，[发现](../../../../../../../doc/design/discovery.md) U10
与[外部取值](../../../../../../../doc/design/liveaccess.md) U2。

这是对插件重构验收（`../acceptance/`）的补充：那边证明机制按设计工作，这里证明一个不知道机制的
人能不能用起来。两边用同一套清河材料（`../table-meta/`、`../sales-semantic/`），本目录不复制业务对象，
只放零知识特有的部分：MySQL 世界、账号与封存答案。本目录不被 construct 写入。

当前实际执行范围已收窄为 **Snapshot 与 Object**，运行方式和证据合同见 [trial/README.md](trial/README.md)。
下文的 MySQL、索引、LiveAccess 世界仍是后续完整业务验收的材料，不在本轮启动或验证。

## Agent 拿到什么

| 给 | 内容 |
|---|---|
| KC | `kc` 可执行文件、服务地址、一个已登录的身份 `zk-colleague`：Dataset `qinghe-analysis` 的 `dataset.resolve,file.read` 与 `feedback.write`，加库存发现 `inventory.read`；没有任何来源仓权限 |
| 数据库 | 一句话：“有 mysql 客户端，连接串是 `mysql://kc_zk_reader:zk-read@mysql:3306/shop`”（试跑容器内经 gateway 到达走查 MySQL） |
| 日期 | 任务用例的原话前固定带“今天是 2026-09-14（周一）” |
| 目标 | 人的原话，见各用例的 `when` |

不给：仓名、Dataset 名、object_id、`doc/`、源码、README、AGENTS，也不能上网查 KC。SQL 与 mysql 客户端
属于环境事实，连接串、账号与日期属于合法外部信息，都不算 KC 的缺口。

取当前值有两条合法路径：经 KC 读外部单元（如表的 `stats`），或用 mysql 客户端直接查询。判定不偏向任何一条，
只要求说清是当前值并给出观察依据或 SQL 与查询时间。KC 负责回答“查哪张表、哪一列、用什么口径、数据从哪来”；
把 KC 里的表对象落到 MySQL 上的物理表，靠表对象的 `qualifiedName`、`platformRef` 与平台、库对象，这一步本身是被测内容。

## 世界

在本节点前态上，operator 做三件事，都不属于 Agent 可见过程：

1. 创建仓 `zk-index`，写入 `../acceptance/sales-analysis/indexes/sales-search/` 中的 `table-name`、
   `metric-text`、`metric-unit` 三个成员。`table-row-count` 不写：它把外部值编入索引，不是这组用例的观察点。
2. 定义并发布 Dataset `qinghe-analysis`，选入下列路径：

   | 仓 | 路径 |
   |---|---|
   | `table-meta` | `tables`、`columns`、`databases`、`database-schemas`、`data-platform-instances`、`data-jobs`、`relations` |
   | `sales-semantic` | `semantic-models`、`metrics`、`relations` |
   | `zk-index` | `indexes` |

   命令形状以 `kc dataset define --help` 为准；两仓的 `relations` 分别交付到 `relations-table-meta` 与
   `relations-sales-semantic`，避免交付目录重叠。另发布对照 Dataset `qinghe-ops`（只选 `table-meta/data-jobs`），
   不给消费身份任何权限。脚本化写法见 `trial/world.sh consumer`。`table-meta/resources`（取值描述）不选入：按
   [外部取值](../../../../../../../doc/design/liveaccess.md)的声明规则，取值描述仍按单元引用的版本解析，只是不进入
   消费者可见范围，所以这不应影响 `stats` 取值。若实际读到取值失败，那是实现与设计不符，按实现缺陷登记，
   不当作本世界的预期。设计原文写的是“没有选入取值描述所在的仓”，本世界是同仓未选路径，按同一精神类推；
   这一点作为设计澄清点随首次运行一并确认。
3. 部署按走查 compose（`repository-attached/_materials/accessor/compose.walk.yaml`），包括 `resource-access`；
   `zk-biz-live-value-states` 的两个变体只改部署，不改 Dataset。在走查 compose 的 MySQL（`127.0.0.1:13306`）上
   以 root 执行 `mysql/accounts.sql`；任务用例再先执行 `mysql/world-base.sql`，后执行一份世界差异脚本：

   | 脚本 | 世界 | 用于 |
   |---|---|---|
   | 无（基础种子） | 6 笔订单、8 行明细，无时间跨度 | zk-env-*、zk-biz-* |
   | `world-base.sql` + `world-real-drop.sql` | 上周 Hangzhou 订单减少，两个 GMV 一致下降 | zk-task-real-drop、zk-task-layered-denial |
   | `world-base.sql` + `world-stale-mart.sql` | 订单不变，`sales_mart` 只到 2026-09-09 | zk-task-stale-mart |

   三套世界共用一个库，每条任务用例开跑前都要按上表重置；场景树的 home 隔离管不到 MySQL。
   `mysql/verify.sh` 在一个加载了基础种子的库上应用账号与两套世界，打印封存答案用到的数字。

## 接入方资料

zk-task-supplier-onboarding 从父节点 `named-repositories-created` 开始：两个托管仓为空，环境里没有清河业务对象。
接入方身份 `zk-supplier` 的权限写明三项：在 `table-meta`、`sales-semantic` 发布对象，定义并发布 Dataset `qinghe-shared`，
给同事授予该 Dataset 的权限。交付物名与同事账号属于人的目标，写在原话里；授权范围因此能收在这一个 Dataset 上。
给接入方 Agent 的是原始文件而不是对象：
表结构导出复用走查 MySQL 的 `accessor/mysql/init/00-schema.sql`，SQL 复用本节点 `../queries/`，口径说明是
`supplier/gmv-caliber.v1.md`。对象组织与命名由接入方自定，按消费方回答的语义验收。
`supplier/gmv-caliber.v2.md` 是 zk-task-feedback-revision 中接入方据反馈修订后的版本，只改第 2 条。

## 人的替身

只回答两类问题：业务目标（例如“我说的 GMV 指对外汇报那个”）与环境外的批准（例如“权限已经批了”）。
其它问题一律答“我不知道，请从环境里找”，同时记一次介入。

## 判定

答案封存在 `answers.yaml`，只给判定者。一条用例通过要同时满足四项：

1. 结果：与封存答案一致；答案写成“允许的说法”的，落在允许范围内即可。
2. 依据：KC 侧给对象、仓、版本或 Dataset 修订；当前值给观察依据，或 SQL 与查询时间。
3. 权限：只用给定的 KC 身份与数据库账号，没有越权、借用或绕过。
4. 信息来源：结论里的每项事实来自环境（KC 或 mysql）或合法介入；除业务目标与环境外批准两类介入外没有别的介入。

允许不同的正确路线：不要求 Agent 按规格 `then` 的先后解题，也不要求走某条命令路径；规格里写到的事实用来
检查依据是否齐全，执行记录只用于解释过程与归因。

一票否决：虚构已执行的命令及结果、对象或字段依据；把零命中当作没有数据；把能搜到当作能读到；靠环境之外的信息继续；
把证据不能区分的原因说成已证实的事实。

探索时出现用法错误并通过公开帮助恢复，记为恢复成本，不等同于虚构执行结果。

## 观察点

下列位置预计容易卡住，但在运行前只是观察点，不预先记为缺口：

- 两个 GMV：技术差别可以从材料推出；该用哪个是人的用途选择，向人澄清是合法介入。
- `enabled: false`：分开看“声明写了什么”与“实际运行如何”。前者在作业定义里；后者环境没有运行记录，只能从数据旁证。
- UNPAID：分开看“计算实际做了什么”与“对外口径怎么定”。前者从表达式与作业 SQL 推出；后者在没有口径对象的世界里环境没写。
- 反馈：入口是 `kc operations feedback record`，需要原访问的 trace。读取帮助与回执没有提到 trace，`kc help consume`
  也没有指向反馈；Agent 能否自己找到“带 `--trace-id` 读取、再按它反馈”这条路，由试跑回答。
- `kc show`：身份有服务范围的 `inventory.read`，看它列出的 Dataset 是否只含有权的一个。

## 完整业务试跑计划（重构后）

先跑三条，每条一个新会话、跑一次；一次完整记录足以说明卡在哪里，稳定性留到有运行器之后再评。

| 试跑 | 规格 | 前态 | 世界 |
|---|---|---|---|
| 认识环境 | zk-env-capability-map，之后同一会话里判定者再问 concepts 判断题 | `qinghe-object-published` | `world.sh consumer`，MySQL 基础种子 |
| 消费 | zk-task-stale-mart | `qinghe-object-published` | `world.sh consumer`，MySQL `stale-mart` |
| 供给 | zk-task-supplier-onboarding，接入方与消费方各一个会话 | `named-repositories-created` | `world.sh supplier`，交付后 `world.sh supplier-finish` |

消费选 stale-mart 而不是 real-drop：real-drop 只靠 mysql 拆分也能答对，测不出 KC 是否起作用；stale-mart 要把人说的
GMV 对应到汇总口径并知道它来自汇总作业，这两条只在 KC 里。评审时同时写出结论里哪些依据来自 KC、哪些来自 mysql。

材料在 `trial/`：

| 文件 | 作用 |
|---|---|
| `world.sh` | operator 搭世界：建 `zk-index`、发布 `qinghe-analysis` 与对照 Dataset `qinghe-ops`、发权、建 MySQL 账号、每次运行前重置 MySQL |
| `compose.agent.yaml`、`Dockerfile.agent`、`agent-entrypoint.sh` | 当前 Snapshot/Object 隔离容器：pi 与同版本 `kc`，经固定身份网关访问 KC 与指定模型；不提供 MySQL、不挂仓库或 Docker socket |
| `prompts/*.md` | 可用工具与人的原话；`snapshot-*` 为当前运行任务，其余为后续业务材料 |
| `run.sh`、`run.py` | 当前 Snapshot/Object 试跑：记录原始事件、身份检查、会话、产物和待评审状态；支持明确的人类追问 |
| `record.md` | 评审记录模板：介入、四项判定、卡点与缺口 |

这组完整业务试跑尚待索引与 LiveAccess 重构完成后接入执行。当前 `trial/run.sh` 只运行
[Snapshot/Object 试跑](trial/README.md)，不执行本节 MySQL 世界。完整事件流保存为 `transcript.jsonl`，
会话副本位于 `sessions/`，结果写在仓库外的 `$KC_ZK_RUNS`（默认 `~/kc-zk-runs`）。

`world.sh check` 里 Dataset 检索不可用时只给警告：这是试跑要记录的观察，不是夹具通过的条件。
世界准备与 MySQL 重置是确定性的，以后可以接到执行器里断言；Agent 的探索与操作留在容器里，不进执行器。

## 缺口登记与归因

每次介入、卡住或失败登记一条：在哪一步、Agent 当时在找什么、答案本应出现在环境的哪里、类别。
失败先记“待归因”，再按顺序排查，查到第一处不成立的就定类：

1. 所需信息在环境里是否存在（系统说明、业务对象）。
2. 从起点能否走到它。
3. 需要的操作是否可用，反馈是否说清了原因。
4. 信息与操作都齐时，Agent 是否推理错误。
5. 环境配置或执行工具是否出错。

| 类别 | 含义 | 由谁修 |
|---|---|---|
| 系统说明缺 | `kc help` 与系统仓都没讲 | KC |
| 说明够不着 | 说明存在，但从起点走不到 | KC |
| 反馈说不清 | 无权、未就绪、插件没装、取值失败、真零命中分不出来 | KC |
| 模型表达不了 | 业务信息在对象模型里没有位置 | KC 设计 |
| 实现与设计不符 | 设计规定的行为没有实现，例如 Dataset 未选取值描述时取值失败 | KC |
| 业务材料缺 | 接入方没写，例如口径、负责人 | 接入方；同时反查 KC 是否引导接入方写 |
| Agent 推理错误 | 信息可达、操作可用，Agent 仍理解或计算错误 | 不算 KC 缺口；多次复现时回看说明是否易误读 |
| 环境或执行故障 | 夹具、部署、网络或工具本身出错 | operator；修好后重跑，不计入结论 |
| 合法外部 | 目标本身、批准、连接串、账号与日期 | 不算缺口 |

“业务材料缺”不直接记到接入方头上：同一消费用例用原始材料与补齐材料各跑一遍。只有原始材料失败、补齐后通过，
才记为业务材料缺，并看 zk-task-supplier-onboarding 中接入方 Agent 是否被环境引导去补这些内容。两遍都失败
不直接判为 KC 缺口，仍按上面的顺序归因。
