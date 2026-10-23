# 索引重构

只有三条任务。不是三批工作，是一次重构的三个部分：

| 任务 | 状态 | 负责什么 |
|---|---|---|
| `INDEX-PUBLISH-DECOUPLE-01` | `doing/` | 发布先接受、索引后台追、进度与就绪可见、对外只有一套 index 词 |
| `INDEX-PHYSICAL-01` | `doing/` | 资源保护与回收、成员与向量产物复用、续页与观察代寿命 |
| `INDEX-SEARCH-SPACE-01` | `doing/` | Dataset 内的检索空间与同修订回读 |

## 为什么只有三条

其余七条已全部并入这三条，原文件在 `done/` 里带 `## 关闭` 写明去向。合并的依据是同批文件与同一前提，
不是名字像：

- 进度可见与分母口径并入 `INDEX-PUBLISH-DECOUPLE-01`：解耦之后"发布返回"不再是"构建完成"，这时候才存在
  可观察的窗口；进度口没有独立于构建调度的宿主。
- 对外词汇、就绪查询、诊断、授权词并入 `INDEX-PUBLISH-DECOUPLE-01`：就绪语义建立在"接受与就绪是两个状态"
  之上，先改名再改语义会在同一批路由文件上返工两次。
- `DEBT-PROJECTION-READY-DIGEST-VERSION-01` 不属于这条线，已移回 `todo/` 顶层：它是就绪快速路径信任既有
  摘要带来的版本守卫问题，与发布时序、对外词汇都无关，只是恰好落在 `declarativeindex/projection/` 目录里。

## 顺序

1. `INDEX-PUBLISH-DECOUPLE-01` 先做。它要改的三篇设计（`dataset.md`、`discovery.md`、`index-physical.md`）
   与 `INDEX-PHYSICAL-01`、`INDEX-SEARCH-SPACE-01` 当前的理解直接冲突，先定下来，另两条才不会按旧顺序
   继续交付。

2. `INDEX-PHYSICAL-01` 与 `INDEX-SEARCH-SPACE-01` 不必等它整项完成，只在两处让路：
   - `INDEX-PHYSICAL-01` 的时序相关文件集（`discovery/datasets/`、`snapshot/dataset/publication_coordination.go`、
     `declarativeindex/projection/dataset*.go`、`index-physical.md` 的准备与发布一节）串行；资源生命周期与
     历史代回收、成员与向量产物复用、配额与清除、真实部署规模验收继续独立推进。
   - `INDEX-SEARCH-SPACE-01` 只在四处判定上随它改变，检索空间语义与全部既有验收一条不减。

3. 三条完成后再轮到消费它们的 Dataset 生命周期任务（`DATASET-REVISION-01`、`DATASET-LIFECYCLE-01`）。
   它们留在 `todo/` 顶层，不属于这条线。`DATASET-PURGE-01` 已于 2026-10-23 并入 `DATASET-LIFECYCLE-01`。

## 判据

- 一个修订被接受，就在文件层可读；检索就绪是同一修订上的第二个状态。两者都是不可变事实，各自可查。
- 每个修订一份独立语料，不共用查询结构。打分语料边界随修订冻结，否则同一修订的检索行为会随别人写入而变，
  归因链断掉。
- 检索默认解析到最新检索就绪修订，并在响应里回报实际使用的 revision；显式指定未就绪修订返回未就绪，
  不静默回退。
- 构建进度按对象计，`done == total` 与 `objectCount` 是两件事，不互相印证。
- 索引对外只有一个入口家族，用户读到的词、路由、字段、授权词都是 index。
