# retrieval/opensearch/

OpenSearch managed projection，属于可丢弃、可重建的检索层，不是 Snapshot authority。

- `index/` 将 Entity、Aspect、Member、Relation 编译为完整的类型化对象文档。
- 本包只装配 OpenSearch provider；旧的 `retrieval/elasticsearch` import path 暂时保留兼容。
- 物理数据使用 generation index；basis、active generation 和状态保存在独立 control index。
- 查询使用 PIT + `search_after`，候选返回后必须在同一 basis 回读 Canonical。

真实容器验证：

```bash
./scripts/e2e-opensearch.sh
```
