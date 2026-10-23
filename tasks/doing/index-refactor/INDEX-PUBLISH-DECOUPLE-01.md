# INDEX-PUBLISH-DECOUPLE-01 · 索引的对外面与构建时序：发布先接受，构建后台追

## 目标

Dataset 发布不再等索引建完，索引的对外能力同时收敛为一套词。

发布请求只做三件事：冻结候选、校验范围与冲突、CAS 接受；返回时新版本在文件层已可读，索引构建作为该版本
的后台任务继续推进。构建期间检索继续服务上一个已就绪版本，检索视图只在目标版本 READY 后前移。维护者随时
能读到这一趟构建的进度，也能在不构建任何东西的前提下问清某个修订现在能不能搜。

- 发布耗时为一次元数据提交，与来源仓对象数无关；大仓发布不再占住请求，也不再撞 CLI 客户端的 30 秒超时。
- 构建期间 SEARCH 永不见半份索引：要么整份旧版，要么整份新版，不混合、不返回部分文档。
- 对外只有一套词：CLI、HTTP 路径、响应字段、授权词用户读到的都是 index，不再出现 projection。
  内部包名与类型名不动。
- `indexes:ready` 回答"能不能搜"，`indexes:status` 回答"这一趟建到哪"，`indexes:describe` 回答"没就绪
  卡在哪"。三者读同一份进度与就绪判定，口径一致，都不触发构建。
- `kc index`、`kc index prepare`、`indexes:ready` 读同一份进度与就绪判定，口径一致。

## 依据

- [Dataset](../../../doc/design/dataset.md#L60-L75)：`vN` 不可改写、`latest` 语义；
  [L90-L98](../../../doc/design/dataset.md#L90-L98) 发布是可恢复的采用决定；[L128](../../../doc/design/dataset.md#L128)
  "准备必须完成才接受新版本"是本条要改的那一句。
- [发现](../../../doc/design/discovery.md#L163-L178)：按冻结来源与选进声明准备索引、搜索只看就绪不触发构建、
  每个实例的进度与失败相互隔离。其中 [L163](../../../doc/design/discovery.md#L163)"全部准备好才接受新版"与
  [L166](../../../doc/design/discovery.md#L166)"搜索永远看不到准备到一半的版本"同批修订。
- [索引物理布局](../../../doc/design/index-physical.md#L89-L91)：成员构建版本与就绪判定沿用；其中
  "Dataset 接受前，检索依据必须已完整、可解析且持续受保护"与三个提交点的顺序是本条要否决的那一句，
  按设计决定 9 修订。
- [Affordance](../../../doc/design/affordance.md)：命令与输出的用户词表。
- [权限](../../../doc/design/permissions.md)、[治理](../../../doc/design/governance.md)：动作词的稳定性与迁移口径。
- [工程质量闭环](../../../doc/engineering/quality.md)。

## 设计决定（实现前必须落到设计篇）

1. 接受与就绪分开。一个版本被接受即对文件读取生效；检索就绪是同一版本上的第二个状态。两者都是不可变事实，
   各自可查。接受不会因为检索未就绪而撤销。
2. `latest` 保持"最新已接受版本"，不因检索未就绪而回退。检索另有一个"最新检索就绪版本"。构建窗口内两者
   可以不同，各自语义明确，且都由版本号表达，不引入第三种名字。
3. 默认 SEARCH 解析到最新检索就绪版本，并在响应 `Dataset` 字段回报实际使用的 revision。显式指定 revision
   且该版本未就绪时返回未就绪，不静默回退。
4. 构建只对已接受版本进行。同一 Dataset 出现更新的已接受版本时，旧目标作废，不为每个中间版本各发一版。
5. 进度 `done` 与 `total` 按对象计，包含被声明排除、不产出文档的对象；`objectCount` 是可检索文档数。
   构建结束时 `done == total`，而 `objectCount` 通常更小；两者不相等、不互相印证。
6. 构建失败不改变已接受事实，只让该版本停在未就绪；检索视图不动。重试由下一次发布或显式 prepare 触发，
   不自动无限重试。
7. 现有 `PublicationProtection` 票据语义随解耦失效。该票据的前提是"准备先于提交"：`Admit` 在提交边界内
   写入准备证据，`Confirm` 在成功后确认（[prepared_resource.go](../../../discovery/datasets/prepared_resource.go)、
   [publication_coordination.go](../../../snapshot/dataset/publication_coordination.go) 的 admit 回调）。提交前移
   之后，票据不再是提交的前置证据，改为：接受时在同一 CAS 内登记一条该版本的构建意图，构建完成后写入就绪
   事实；崩溃恢复按意图重放，不按票据恢复。这一条不写清就实现，会得到"票据已确认但索引不存在"的错乱状态。
8. HTTP 与 CLI 走同一条路径。现状不一致：HTTP 经 `resources.PreparePublication` 调
   [Preparation.Execute](../../../discovery/datasets/prepare.go)（无票据），CLI 经 `ExecuteProtected`（有票据）。
   解耦后两者共用"接受 → 登记意图 → 后台构建"这一条，不再有有无票据两个变体。
9. `INDEX-PHYSICAL-01` 的物理布局结论保留，时序前提被本条否决。它的「实施前待定」已收敛为
   "每版独立语料、不共享查询结构"，与本条一致，不再待审；但
   [index-physical.md](../../../doc/design/index-physical.md#L89-L91)"Dataset 接受前，检索依据必须已完整、
   可解析且持续受保护"以及"引擎生成代提交 → Dataset 接受 → 引用保护交接"的三提交点顺序与本条相反，
   须同批修订为"接受 → 意图登记 → 后台构建 → 就绪事实"。它的资源保护与回收、成员与向量产物复用、
   查询与续页保护、历史代回收与本次反转无关，不受影响，继续按原验收推进。
10. 与 `INDEX-PHYSICAL-01` 改同一批文件，不能并行。本条后开，按 [tasks/README.md](../../README.md) 的规则
    先停；开工前把设计决定 9 的前提修订登记进该任务的事实记录，不另立重复任务。
11. `INDEX-SEARCH-SPACE-01` 的空间语义全部保留，但有四处判定随本条改变，须同批登记，不改其既有验收：
    - "corpus 丢失后不能自动恢复"从缺陷变成必然。解耦后语料是构建产物，未就绪与丢失用同一个状态表达、
      同一入口重试，不再需要"自动恢复到原代"这条独立能力；该缺口的判定改为"丢失后版本停在未就绪且可重试"。
    - "单个成员失败时整次准备被拒"的判定落点变。接受已先发生，成员失败只让该版本停在未就绪并记录失败
      单元，不再回滚发布；逐成员隔离的验收内容不变，失败的外在表现从"整次拒绝"改为"部分就绪不可见"。
    - "多索引实例时成员选择未实现"不受影响，仍按它自己的验收。
    - `accept-search-first-revision-not-ready`
      （[accept-09](../../../.data/scenes/service-initialized/named-repositories-created/qinghe-object-published/_specs/accept-09-search-space.yaml#L99-L108)）
      的 given 写着"测试部署暂停构建"，但仓内没有任何暂停构建的开关，这条现在不可构造。解耦后不自造
      暂停钩子：改用"发布返回即已接受、构建未完成"这个真实窗口产生未就绪，并补一条反向用例
      "文件读取在构建期间就已可用"，证明两态不是同一状态。
12. 对外词汇一次改净。`kc index` 家族、`indexes:*` 路由、`searchView.indexRevisions`、授权词
    `index.read`／`index.manage` 在同一次交付内切换，不留 `projection.*` 过渡期、不留旧命令别名。
    理由是留着双词会让"索引的解耦"和"索引的改名"两条迁移互相等待；一次改净只需一次 allow 文件与场景
    授权迁移。
13. `ready:false` 是正常答案，不是故障：HTTP 200，无 `objectCount`，配错误码反而要求调用方把正常状态
    当异常处理。`BUILDING`、`UPDATING`、`FAILED`、`RETIRED` 与从未构建在这一层都收敛为 `false`，
    不把内部状态名做成对外字段。

## 用例（先行：实现前必须先落这些用例并记录当时的颜色）

用例写成场景 `_probes`（观察走 `kc` / typed HTTP）与具名 Go evidence，接入最近宿主。每条只负责一种失败风险。

可见性

1. 发布返回耗时与来源对象数无关：构造一个构建远未完成的来源，发布仍返回，且返回耗时不超过固定阈值。
   该用例直接覆盖 CLI 远端路径的 30 秒超时失效。
2. 发布返回后 `kc dataset show` 显示新版本已接受，且立即 `kc read` 能读到本版内容。
3. 第 1 条的发布返回时立即查 `indexes:ready` 得 `ready:false`，HTTP 200；同一时刻索引文档数与进度均无
   增长（证明查询不触发构建）。
4. 同一时刻默认 SEARCH 返回上一版结果集，不命中本版新增对象，响应 `Dataset.revision` 为上一版。
5. 构建完成后默认 SEARCH 命中本版新增对象，`Dataset.revision` 为新版；`index --revision N` 报已就绪且对象数。
6. 构建失败时默认 SEARCH 仍返回上一版，新版 `ready:false`，`dataset show` 标出未就绪，已接受事实不变，
   文件读取照常。
7. 显式指定未就绪 revision 的 SEARCH 返回未就绪，不回退到上一版。

进度

8. 构建中 `indexes:status` 返回 `repository`/`done`/`total`；`done` 单调递增，`total` 同趟内稳定。
9. `total` 未知时只打印 `repo  done`，不出现占位分母。
10. 构建成功或失败后 status 回落 `{}`，不残留上一趟数字。
11. 未配置索引实例时 status 返回 `CAPABILITY_UNSATISFIED`，不退化成"没有构建"。
12. 进程重启后未完成构建从 0 重计，不跨重启累计。
13. `kc index prepare` 进行中与同时刻 status 同 `repository`；两者不同名时显示"准备中"占位，不显示别人的数字。
14. 用含被声明排除对象的夹具，构建结束断言 `done == total` 且 `objectCount < total`，证明分母含被排除对象。

无冲突

15. 两个维护者并发发布同一 revision：一个成功、一个 `NON_FAST_FORWARD`；被接受版本不被覆盖，版本顺序单调。
16. 并发发布 vN 与 vN+1（后者基于前者）：不出现半份清单，最终接受较高版本且满足前者的可见范围。
17. vN 已接受仍在构建时发布 vN+1：vN 构建作废且不覆盖任何已接受版本，vN+1 正常进入并最终就绪；
    期间默认 SEARCH 始终服务某个完整版本。
18. 构建期间连续 SEARCH 的结果集要么全属上一版、要么全属新版，不存在含新版部分文档的中间响应。
19. 相同定义重复发布返回 `REPLAYED`，不产生新版本，也不触发第二次构建。
20. 撤权后下一次发布被拒绝，已接受版本与在跑构建的收尾不受影响。
21. 同一 Dataset 两个版本构建并行时，各自就绪后对象数与对象集合互不串。
22. 构建期间对上一版的 SEARCH 正常返回，不报未就绪、不被构建阻塞。
23. 接受后进程被杀：重启后该版本仍为已接受，检索视图未前移（新版本未就绪），构建按意图重新开始并最终就绪；
    不存在永久停在未就绪且无重试入口的状态。
24. 构建意图与已接受版本一一对应：重启恢复后不出现同一版本重复构建出两份物理索引，也不出现意图丢失导致
    永不构建。

归因

25. 同一版本重复查询，结果集与顺序稳定，不因其它版本的构建而变化。
26. 版本只覆盖其 pin 范围：来源仓在 pin 之后的新增对象不出现在该版本的检索结果里。
27. 检索结果回报的 `Dataset.revision` 与实际参与打分的语料版本一致：不出现"报新版、按旧版语料打分"。

对外面

28. `kc index --dataset qinghe-analysis --revision 5` 打印 `qinghe-analysis 第 5 版  已就绪  120 个对象`
    或 `qinghe-analysis 第 5 版  尚未就绪`，且调用前后索引文档数与构建计数零增长。
29. `kc index --revision 5`（无 `--dataset`）返回 `USAGE_INVALID`；`kc index`（无 `--dataset`）走进度，
    不是就绪。
30. 省略 `--revision` 时 `indexes:ready` 返回的 `revision` 等于当前服务版，不是请求回显；重复调用得到
    同一坐标。已就绪时 `objectCount` 与既有描述接口的对象数一致。
31. `kc operations projection describe|sync|notice` 三条旧 argv 全部被拒，`kc --help` 与运维分组帮助里
    不再出现 projection 字样。
32. `/operations/v1/indexes:ready`、`:prepare`、`:describe`、`:status` 返回 200；
    `/operations/v1/projections:*` 与 `/operations/v1/indexes:notice` 均不在路由闭集。
    `routes_golden.txt` 与闭集一致：删 `projections:describe`、`projections:sync`，加
    `indexes:ready`、`indexes:prepare`、`indexes:describe`、`indexes:status`，共 90 条。
33. 搜索响应只含 `searchView.indexRevisions`，无 `projectionRevisions`。
34. 授权：`index.read`／`index.manage` 切换后仓内全部 allow 文件与场景授权仍能通过，或随同一次改动迁移
    干净；不存在只改文档、代码仍认旧词的中间态。拒绝信息里的动作名是用户能在 help 里读到的词。
35. 只有 `file.read` 的调用者可用 `indexes:ready` 与 `indexes:describe`；超出 Dataset 发现范围时拒绝；
    `server/repository_readiness.go` 的检索状态判定不因改词而放宽或收紧。
36. `indexes:describe` 能回答四个问题：哪些成员未就绪、各自处于什么阶段、basis 指到哪个提交、是否曾失败；
    不触发构建、不推进来源 HEAD；对外字段有明确的用户词映射，不暴露内部状态名。
37. 日常 `indexes:ready` 的成本不因 `indexes:describe` 存在而上升（同一 READY 修订上，前者不触发成员全量核对）。

## 验收

- 上面 37 条全部接入实际 probe 或具名 Go evidence 并跑绿，无 skip。
- 用例先行且颜色有记录：实现前逐条实测并记录，不得凭推断填色。已知预期为红的至少包括 1、3、4、7、17、18、
  22、23、24、27、28、31、32、33、34（现状为请求内同步全量构建、且对外仍是 projection 词汇）；预期为绿
  或部分绿的包括 2、5、8–14、19–21、25、26、29、30、35–37。任一条与预期不符，先解释再改，不改预期去
  迁就实现。
- HTTP 与 CLI 发布走同一条路径：证明 `Preparation.Execute` 与 `ExecuteProtected` 不再产生有无票据两个变体
  （设计决定 8）。
- 设计篇改完并重读：[dataset.md](../../../doc/design/dataset.md) 的发布一节（L90-L98、L128，以及 L69
  「`latest` 永远指向一个已经准备好的版本」这句随设计决定 2 修订）、"跟随"一节中「准备必须完成才接受」
  与「拆成两个 Dataset」的变通；[discovery.md](../../../doc/design/discovery.md) 的索引维护一节（L163、L166、
  `done`/`total` 口径，以及能力表里"Dataset 发布前准备"改为"发布后按需构建、进度可见"）；
  [index-physical.md](../../../doc/design/index-physical.md#L89-L91) 的三提交点顺序与其 mermaid 图
  （`J --> K` 一条须改为接受在构建之前，`M` 的引用接管改为就绪事实登记）；
  [affordance.md](../../../doc/design/affordance.md) 的 U5、U7 里"未就绪"由短窗口变为常态窗口后，
  交互路径改为"构建中，可查进度"；[service.md](../../../doc/design/service.md#L28) 的跨模块编排句
  "先准备索引再接受"改为"接受后构建"；[concepts.md](../../../doc/concepts.md) 补"就绪"词条，
  与"发布"并列。`discovery.md` 里"物理上不要求每个版本一份索引、多个版本可以共用物理数据"一句须收敛为
  "打分统计与语料边界随版本冻结，不共用统计学意义上的语料"。
- `INDEX-PHYSICAL-01` 的剩余范围不因本条作废：其资源保护与回收、成员与向量产物复用、查询与续页保护、
  历史代回收等验收照旧。本条结束时须在该任务事实记录里留下"时序前提已由本条反转"的登记，
  不留读者按旧顺序理解其后续交付。
- `INDEX-SEARCH-SPACE-01` 的空间语义验收一条不减（`accept-08`、`accept-09`、`accept-13` 与
  `DATASET_INDEX_SPECS` 六条照旧）。本条结束时须在该任务事实记录里登记设计决定 11 的四处判定变化；
  其中 `accept-search-first-revision-not-ready` 改用真实构建窗口构造，不再依赖不存在的暂停开关。
- `make check-docs`、`make quality`、`make check-validation`、`make test` 通过；受影响包定向回归绿。
- 真实部署验收另记：`make deploy-local-scenes` 只证明发布后检索最终就绪，不代替上面 37 条。

## 前置

与 `INDEX-PHYSICAL-01` 按文件集串行，不按整任务串行。它不是可并行的前置，也不该被整项阻塞。

冲突文件集（双方都会改，同一时刻只能一条线动）：

- `discovery/datasets/dataset_publish.go`、`prepare.go`、`prepared_resource.go`
- `snapshot/dataset/publication_coordination.go`
- `declarativeindex/projection/dataset.go`、`dataset_resource.go`、`dataset_state.go`
- `doc/design/index-physical.md` 的准备与发布一节

`INDEX-PHYSICAL-01` 的其余范围不冲突，可独立推进，不在本条等待之内：`opensearchimpl` 的资源生命周期与
历史代回收、`computation_cache.go` 的成员与向量产物复用、配额与清除、真实部署规模验收。这些都不依赖
提交点顺序，各自按原验收走。

`INDEX-PHYSICAL-01` 当前在 `doing/`。按真实依赖，本条应先做、它后做——它已交付的发布收据、采用收据绑定与
提交准入都建立在"准备先于提交"之上，顺序反转后这批要重做；让它先跑完再反转，等于先做一遍再拆一遍。
所以处理方式是：本条开工即先做这条前提修订（把设计决定 9 写进它的待定项并落设计篇），它不必停整项，
只需在时序相关文件集上让路。开工第一步：把设计决定 9 的前提修订写进 `INDEX-PHYSICAL-01` 的事实记录。

`INDEX-SEARCH-SPACE-01` 判定有变化，不能按"只改调度不改判定"处理。它的检索空间语义（Dataset 是唯一
空间、只搜索选进声明、固定修订不跟随 HEAD、消费权不替代仓读权、撤权拒续页）全部保留；随本条变化的是
两条剩余缺口的判定口径与一条验收用例的可构造性，见设计决定 11。它的"进展（要点）"里 MVP 链路已交付
的是同步路径：空间语义留存，时序被本条替换，须在其事实记录里登记。

`READ-BASIS-01` 无冲突：固定读取依据的交付是本条构建期的输入，本条不碰它。

## 并入

- `INDEX-CATCHUP-01`：解耦之后必须能看进度，是同一件事的另一面。`indexes:status` 与 `kc index` 的进度
  输出按本条验收 8–14 执行。原"不把 `indexes:prepare` 改成异步作业"一条以"保持同步语义"的形式保留。
- `DEBT-INDEX-PROGRESS-DENOMINATOR-01`：分母口径已写成本条设计决定 5 与验收第 14 条。
- `INDEX-SURFACE-01`：对外词汇、命令家族、就绪查询、诊断与授权词一次改净，见设计决定 12、13 与验收 28–37。
  它与本条的发布时序共用同一批 `server/` 路由与 `cli/` 声明文件，且它的就绪语义完全建立在"接受与就绪是
  两个状态"之上——分开做要先在旧同步语义上改名再返工。
- `INDEX-READY-01`：窄就绪查询，见设计决定 13 与验收 28–30、35。
- `DEBT-INDEX-READY-DIAGNOSE-01`：诊断由 `indexes:describe` 承接，处置方向选定"复用既有完整描述结构、
  只改路径与对外词汇"，见验收 36、37。
- `DEBT-INDEX-AUTH-VOCAB-01`：授权词直接切换、不留双词，见设计决定 12 与验收 34。

以上六条原文件已移入 `done/` 并各自写 `## 关闭`，写明并入本条与承接位置。

## 不做

- 不改构建算法：本条仍每版全量重建，只改调度、可见性与对外词汇。克隆加差集增量、向量可复现等价性另立
  任务，需先实测 provider 的索引克隆能力与 HNSW 增量等价，不在本条夹带。
- 不改版本号分配：仍由调用方传入作为幂等校验，服务端分配见 `DATASET-REVISION-01`。
- 不做 repo 级检索，不为 repo 语境保留任何独立语料或就绪语义。
- 不把诊断字段塞回 `indexes:ready`，不给 `ready:false` 配 `reason` 字段或错误码。
- 不保留 `projection.*` 授权词过渡期，不给旧命令留隐式别名或等价性回退，不新增 `indexes:notice`。
- 不改内部包名、类型名与持久状态键：`discovery/projection`、`declarativeindex/projection`、
  `ProjectionRequest` 是实现词。
- 不承诺进度跨重启累计，不保存速率或预计完成时间，不做取消构建入口。
- 不让 `latest` 因未就绪而回退或跳过版本；接受顺序是唯一的版本顺序。
- 不为"检索滞后窗口可能很长"引入自动降级或静默回退；滞后靠 status 可见，缩短靠另立的增量任务。
