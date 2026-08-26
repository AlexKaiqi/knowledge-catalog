# knowledge/

② Knowledge protocol：`object_id`、Address、Aspect、Schema、provenance、Binding，以及 PUT/REMOVE、READ/RESOLVE/LOG/DIFF。

本包直接拥有 `ObjectID`、`Address`、`KnowledgeRef`、Schema ref 与 provenance 类型；它们不是为了少一个 import 而放进 `kernel/` 的“共享类型”。这让 ⓪ Snapshot 和 ① Catalog 在类型层面也无法携带知识语义。

`Repository` 是 `knowledge/reader` 在 Snapshot `TreeStore` 之上构造的只读②视图，不是 Snapshot Adapter 的可选接口。挂载普通 Repository 只要求 `snapshot.Store`；结构化消费由 Reader 在应用装配接缝上显式解释固定 tree。

`Repository` 只提供精确读、历史与 List，不提供写入或 `Search/Probe/Retrieve`。PUT/REMOVE 只进入 Writer；生产候选定位只走 `index.Retriever`，因此 Snapshot adapters 不复制知识读写或检索逻辑。

`knowledge/writer` 接收 Knowledge `ChangeSet`；⓪ `snapshot.Store` 不接收 PUT/REMOVE。Writer 在唯一的②→⓪接缝上把 ChangeSet 编译为字面 `TreeChangeSet`，再调用 `snapshot.TreeStore`。`knowledge/reader` 同样只从固定 commit 的 path/blob 解释 Canonical。具体 Adapter 不实现 `knowledge.Repository`，也不解析 frontmatter。

Binding 只保存稳定声明；运行状态、cursor、watermark、凭证和实际 endpoint 留在墙外 Materialization Runtime。

目录内聚合整个②垂直面：根包拥有协议类型与只读 Repository 视图，`reader/` 负责固定 commit 上的知识解释，`writer/` 负责 Knowledge ChangeSet 的 COMMIT/PROPOSAL。字面路径写入不在本目录，见 `snapshot/treewriter/`；检索合同不在本目录，见 `retrieval/`。
