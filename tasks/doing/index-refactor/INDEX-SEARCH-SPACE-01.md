# INDEX-SEARCH-SPACE-01 · Dataset 内的对象检索按修订可用

## 目标

在一个 Dataset 的某个已发布修订内，对象可以被搜索到，并按同一修订读回。

一句话：选进 Dataset 的索引声明按成员建索引，查询只在这个 Dataset 的这个修订里找对象。

## 依据

- [发现](../../../doc/design/discovery.md)：检索空间、完整性、访问原语、索引维护。
- [Dataset](../../../doc/design/dataset.md)：发布、固定交付。
- [LiveAccess](../../../doc/design/liveaccess.md)、[权限](../../../doc/design/permissions.md)：外部取值与消费授权。

## 验收

- `accept-08-index-build`、`accept-09-search-space` 全绿。
- `.data/scenes/accept.py` 的 `DATASET_INDEX_SPECS` 六条在测试部署执行为绿：`accept-index-writer-accepts-unsupported`、`accept-index-array-input`、`accept-search-unsupported-member-isolated`、`accept-search-vector-member`、`accept-index-catalog-version-ahead`、`accept-index-catalog-upgrade-recheck`。
- `accept-cross-shared-only-indexed`、`accept-cross-hit-recheck`、`accept-cross-retained-revision-not-refreshed` 变绿。
- 检索侧反例：Dataset 消费权先于候选查询、选入文件范围先于候选查询、清单外对象不出现在结果里。
- 受影响包回归为绿：`go test ./discovery/... ./declarativeindex/... ./invocation/... ./object/reader/...`。
- `make check-docs`、`make quality` 通过。

## 剩余缺口

- 索引构建只能按对象增量，不能按成员（一条索引声明）重建。
- 单个成员不支持或外部观察失败时，整次准备被拒；需要逐成员隔离并说明失败单元。
- 多个索引实例时，成员选择未实现。
- corpus 丢失后不能自动恢复。

## 前置

`INDEX-DECLARATION-01`、`LIVEACCESS-READ-01` 的代码与 `make test` 已绿；真实部署验收到环境恢复后补跑。

`CONSUME-BASELINE-01` 与本任务改同一批 `invocation/`、`discovery/` 文件，同一条线推进。

## 不做

- 不碰其他检索范围。`--repo`、调用者可读全仓的存废由各自范围的任务裁决，本任务不删除、也不新增该范围的拒绝断言。
- 不做跨 Dataset 复用分段。
- 假定一个对象的各个方面在同一仓；同一 `object_id` 分散在两个仓的组合路径不进本轮验收。已实现的组合行为与其回归保留。

## 进展（要点）

MVP 链路已交付并验证：发布 Dataset、Dataset 内搜索、同修订回读与文件下载、跨仓同对象组合、外部观察按 `shared` 入索引并在回读后补判、观察期间发布新修订拒绝候选。详细过程与逐轮门禁记录见 git 历史。

关键证据：
- `.validation/runs/20261016-mvp-last-mile/summary.json`
- `.validation/runs/20261016-doing-validation/summary.json`
- `.validation/runs/20261016-index-search-space/revision-publication-{red,green}.txt`

已并入并关闭的债：
- `DEBT-DATASET-STATE-ACCEPT-ATOMIC-01`：修订围栏已交付（`snapshot/dataset/revision_fence.go`），同进程窗口闭合。
- `DEBT-OPENSEARCH-CONTROL-STARTUP-503-01`：控制索引 `RECOVERING` 的有界只读等待已实现（`declarativeindex/opensearch/internal/opensearchimpl/control_readiness.go`）。
- `DEBT-CLI-SURFACE-CONTRACT-01`：`cli/SURFACE.md` 旧段落已同步。
- `DEBT-STATE-OBSERVER-RETIRED-ROUTE-01`：与检索无关，已剥离为 `DEBT-OBSERVER-RETIRED-NOTICE-01`。

## 判定随发布解耦改变（2026-10-23）

`INDEX-PUBLISH-DECOUPLE-01` 反转了发布时序。本任务的检索空间语义一条不减（Dataset 是唯一空间、
只搜索选进声明、固定修订不跟随 HEAD、消费权不替代仓读权、撤权拒续页），`accept-08`、`accept-09`、
`accept-13` 与 `DATASET_INDEX_SPECS` 六条照旧。改变的是以下四处判定，已按新时序登记：

- corpus 丢失后不能自动恢复：从缺陷变成必然。语料是构建产物，未就绪与丢失用同一个状态表达、同一入口
  重试；判定改为「丢失后版本停在未就绪且可重试」，不再要求「自动恢复到原代」。
- 单个成员失败时整次准备被拒：判定落点变。接受已先发生，成员失败只让该版本停在未就绪并记录失败单元，
  不再回滚发布；逐成员隔离的验收内容不变，外在表现从「整次拒绝」改为「部分就绪不可见」。
- 多索引实例时成员选择未实现：不受影响，仍按本任务自己的验收。
- `accept-search-first-revision-not-ready`（`accept-09-search-space.yaml`）：given 里写的「测试部署暂停构建」
  在仓内没有任何对应开关，旧写法不可构造。已改用真实窗口——发布返回即已接受、构建未完成——产生未就绪，
  并补一条反向用例证明「文件读取在构建期间就已可用」，不新增暂停钩子。

## 接管（2026-10-16）

本线曾停滞超过 20 分钟，按接管规则由 Codex 接续；当时结论与证据保留在上文证据目录。
