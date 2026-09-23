# retrieval/opensearch/

OpenSearch managed projection，属于可丢弃、可重建的检索层，不是 Snapshot authority。

- `index/` 将 Entity、Aspect、Member、Relation 编译为完整的类型化对象文档。
- 本包拥有 OpenSearch provider 的完整实现；不提供 Elasticsearch 兼容路径或配置别名。
- 物理数据使用 generation index；basis、active generation 和状态保存在独立 control index。
- 查询使用 PIT + `search_after`，候选返回后必须在同一 basis 回读 Canonical。
- Open 仅校验本地配置，不发送网络请求；首次重建才确保 control index。搜索、关系及 PIT 的超时、
  部分分片失败和提前终止不得变成完整候选；增量每批等待其触及分片可见后才发布。
- 物理版本 `opensearch-v4-exact-temporal-keys` 使用无损整数游标与纳秒时间排序键；时间键只属于
  adapter，公开排序元数据使用规范时间。既有物理版本通过原维护机制重建。
- 每次候选读取核对活动 generation/basis；创建 PIT 前后复核控制文档的 CAS 版本，拒绝与增量或
  发布交错的读取。已失效的游标要求重启查询，不跟随新 basis。
- Workspace 不进入文档 mapping。上层按 ResolvedKnowledgeSet 的固定
  `(repository, commit)` 选择 generation 并扇出；多 index/`_msearch` 或绑定不可变 PinID 的
  短期 alias 只允许作为可丢执行优化。

真实容器验证：

```bash
./scripts/e2e-opensearch.sh
```

## 语义向量投影

设置 `KC_EMBEDDING_MODEL` 与 `KC_EMBEDDING_DIMENSIONS`（加 OPENAI_BASE_URL/OPENAI_API_KEY）后，
投影在生成索引里加入 `semantic_vector` knn_vector 字段（hnsw/lucene，cosinesimil），构建批次内
以一次 embeddings 调用为每个对象正文派生向量；无正文对象（如关系）不入语义窗口。模型或维度
未配置的引擎保持纯 lexical 投影。语义窗口是单次有界 top-K 查询，候选携带 approximate 证据；
这是派生投影，不是第二权威（docs/RETRIEVAL.md §8.1）。

