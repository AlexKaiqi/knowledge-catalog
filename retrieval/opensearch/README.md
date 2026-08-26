# retrieval/opensearch/

OpenSearch managed projection，属于可丢弃、可重建的检索层，不是 Snapshot authority。

- `index/` 将 Entity、Aspect、Member、Relation 编译为完整的类型化对象文档。
- 本包拥有 OpenSearch provider 的完整实现；不提供 Elasticsearch 兼容路径或配置别名。
- 物理数据使用 generation index；basis、active generation 和状态保存在独立 control index。
- 查询使用 PIT + `search_after`，候选返回后必须在同一 basis 回读 Canonical。
- Workspace 不进入文档 mapping。上层按 ResolvedWorkspace 的固定
  `(repository, commit)` 选择 generation 并扇出；多 index/`_msearch` 或绑定不可变 PinID 的
  短期 alias 只允许作为可丢执行优化。

真实容器验证：

```bash
./scripts/e2e-opensearch.sh
```
