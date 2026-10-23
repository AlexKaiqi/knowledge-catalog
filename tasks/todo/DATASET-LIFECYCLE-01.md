# DATASET-LIFECYCLE-01 · Dataset 修订的生命周期：保留、退役、准入与清除

## 目标

一个修订从冷到热再到退役，检索、回收、发布准入与紧急清除都只看同一份状态机。

检索保留按“至少保留多长时间且至少保留几个修订”执行；超出保留的修订可读不可搜，搜索明确报检索已不可用，
不返回零命中；被替换修订的续页只在宽限内可用，过后明确要求重新开始。退役修订读取与搜索都拒绝，错误与
“检索已不可用”可区分，发布历史与来源提交仍可追溯；当前修订不能退役。无人访问和任务固定都不改变保留。
测试部署提供可控时钟。

未退役修订持有其冻结的来源提交，lakeFS 回收不删除它们；退役后保留释放。回收规则会越过保留时部署拒绝
启动，不潜伏到读取时才报错。按 Dataset 设配额，超出时自动发布与手动发布都不接受新修订，当前修订照常服务。

紧急清除在有界时间内重写该修订自己的物化语料：证据中的观察值替换为“已清除”，摘要与访问记录保留，
清除状态给出完成时间。清除不删除使用记录本身，不改修订的可读性。每个修订一份独立语料，所以清除范围是
“这个修订的这份语料”，不会影响其他修订的检索。

## 依据

- [Dataset](../../doc/design/dataset.md)：U12 修订的检索保留与退役、U13 来源提交不被回收、U14 配额、U15 紧急清除。
- [发现](../../doc/design/discovery.md)：索引维护、续页。
- [LiveAccess](../../doc/design/liveaccess.md)：观察证据的清除。
- [Snapshot](../../doc/design/snapshot.md)。

## 验收

- [accept-12-dataset-revisions](../../.data/scenes/service-initialized/named-repositories-created/qinghe-object-published/_specs/accept-12-dataset-revisions.yaml)
  中以下规格全绿：`accept-revision-search-retention`、`accept-revision-continuation-grace`、
  `accept-revision-retire`、`accept-revision-retire-current-rejected`、`accept-revision-usage-no-auto-retire`、
  `accept-revision-pinned-task-out-of-retention`、`accept-revision-source-commit-held`、
  `accept-revision-gc-rule-check`、`accept-revision-quota`、`accept-revision-emergency-purge`。
- 物理回收消费 `INDEX-PHYSICAL-01` 已交付的资源登记机制，不重建保留策略：未满保留的旧修订检索不得被
  提前回收，须有回归证明。进程内三分钟删除定时器已由该任务移除，本任务只把“何时可回收”接上退役判定，
  不新造第二套期限。
- 清除后仍能读到修订与对象；证据的摘要与访问记录存在，观察值位置读到的已是“已清除”。清除只重写该修订
  自己的语料，同一对象的其他修订语料不受影响，也不需要重写。清除有明确完成时间，重复清除不报错也不重复
  改写。清除须在有界时间内完成，不做成异步后台任务。
- `make deploy-local-scenes` 通过（回收、提交保留与语料重写都需要真实 lakeFS 与搜索容量）。
- `make check-docs`、`make quality`、`make test` 通过。

## 前置

- `INDEX-SEARCH-SPACE-01`：保留、回收与清除的对象都是按 Dataset 修订的检索空间。该任务的空间语义已交付；
  其中“语料丢失后不能自动恢复”一条的判定随 `INDEX-PUBLISH-DECOUPLE-01` 改为“丢失即未就绪、可从同一
  入口重试”，本任务按改后的判定验收，不要求恢复到原代。
- `DATASET-REVISION-01`：`accept-revision-quota` 含自动发布路径。
- `INDEX-PHYSICAL-01`：物理资源登记与回收接缝。
- `DISCOVER-USAGE-01`：`accept-revision-usage-no-auto-retire` 里“使用数据逐修订列出访问时间与空间”的断言
  由它提供数据面。本任务只按“使用数据不得自动触发保留或退役”验收，不实现使用数据本身；该任务未完成前，
  这条规格只能验到“不自动退役”这一半，不把它悄悄降级成后半句。

## 不做

不按使用情况自动保留或退役；不做来源跟随（`DATASET-REVISION-01`）；不清除来源仓历史；不做跨仓清除；
不做跨仓事务；不删除使用记录与访问账。

## 沿革

2026-10-16 从 `DATASET-REVISION-01` 拆出 U12，原因为跟随会持续累积修订，回收需要独立判定。
2026-10-16 又拆出 U13/U14 为 `DATASET-STORAGE-01`。2026-10-22 从后者拆出 U15 为 `DATASET-PURGE-01`。

2026-10-23 把 `DATASET-STORAGE-01` 并回本条，改名为 `DATASET-LIFECYCLE-01`。并入理由：U13“未退役修订
持有来源提交”与 U14“超配额不接受新修订”都以 U12 引入的“修订已退役”状态为唯一判定输入，且三者改同一批
文件——修订状态机、发布准入与回收触发点。原 `DATASET-STORAGE-01` 自己的“前置”就写着保留是它的直接前置、
“前两项共用修订退役即释放的同一实现”。分两条时，第二条要先等第一条把状态机定稿，同一处代码改两遍。

2026-10-23 把 `DATASET-PURGE-01` 并回本条。原先拆出的理由是“紧急清除触及共用索引重写，与来源提交保留、
配额没有共同实现”。该理由已被 `INDEX-PUBLISH-DECOUPLE-01` 推翻：那里把结论收敛为“每个 Dataset 修订一份
独立语料，不共用查询结构”，`DATASET-PURGE-01` 自己的目标一节也已按这个结论写。共用索引既不存在，清除与
保留就落在同一份语料上——退役释放它、清除重写它，都读同一个修订状态，也都由同一个回收接缝执行。
分两条时，同一处语料处置要改两遍。合并后一句话能说清：一个修订的语料，何时可回收、被回收、被重写，
都只看同一份状态。

同批修正两处：

- 原验收“旧代际固定保留 3 分钟即删除的逻辑改为按保留策略回收”已失效：该定时器由
  `INDEX-PHYSICAL-01` 移除（见 `declarativeindex/opensearch/README.md` 与
  `tasks/doing/index-refactor/INDEX-PHYSICAL-01.md`“已移除进程内三分钟删除定时器”）。改为证明未满保留
  的修订不被提前回收，不再要求改写不存在的代码。
- 原前置 `INDEX-USAGE-01` 已被删除。2026-10-23 该任务按原文恢复为 `DISCOVER-USAGE-01`，本条前置改指它，
  并把“规格只验到一半”的事实写清，不把断言悄悄降级。
