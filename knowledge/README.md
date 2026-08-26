# knowledge/

② Knowledge protocol：`object_id`、Address、Aspect、Schema、provenance、Binding，以及 PUT/REMOVE、READ/RESOLVE/LOG/DIFF。

本包直接拥有 `ObjectID`、`Address`、`KnowledgeRef`、Schema ref 与 provenance 类型；它们不是为了少一个 import 而放进 `kernel/` 的“共享类型”。这让 ⓪ Snapshot 和 ① Catalog 在类型层面也无法携带知识语义。

`Repository` 是 Snapshot 成员的可选②能力。挂普通 Git 只要求 `snapshot.Store`；Reader、Writer 或 Index 在应用装配接缝上通过 `knowledge.Of` / `knowledge.Lookup` 明确要求知识解释能力。

`Repository` 只提供精确读、历史、List 与 PUT/REMOVE，不提供 `Search/Probe/Retrieve`。调试 contains 由 Reader 基于 List 完成；生产候选定位只走 `index.Retriever`，因此 Snapshot adapters 不需要各复制一份检索逻辑。

`ApplyKnowledgeCommit` 接收 Knowledge `ChangeSet`；⓪ `snapshot.Store` 不接收 PUT/REMOVE。Writer 在②→⓪接缝上优先调用 adapter 的原生实现；普通 Snapshot 若暴露可选 `snapshot.TreeStore`，则由 Writer 的共享 repofile codec 编译为字面 `TreeChangeSet`。因此 PUT/REMOVE 不会下沉进⓪接口，纯树 adapter 也不必复制知识协议。

Binding 只保存稳定声明；运行状态、cursor、watermark、凭证和实际 endpoint 留在墙外 Materialization Runtime。

目录内聚合整个②垂直面：根包拥有协议类型与 Repository capability，`reader/` 负责固定 commit 上的知识解释，`writer/` 负责 Knowledge ChangeSet 的 COMMIT/PROPOSAL。字面路径写入不在本目录，见 `snapshot/treewriter/`；检索合同不在本目录，见 `retrieval/`。
