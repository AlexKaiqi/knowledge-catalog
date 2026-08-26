# retrieval/elasticsearch/（兼容路径）

OpenSearch managed Projection 的历史 import 路径；不是权威 Store，也不保存可直接返回给消费者的知识正文。新代码优先使用 `retrieval/opensearch`。

| 文件 | 负责 |
|---|---|
| `config.go` | 非密配置、环境凭证策略、Engine opener |
| `client.go` | HTTP transport、认证头、control/data mapping |
| `projection.go` | generation、CAS control、Bulk、Rebuild/Apply/Count 与物理文档编码 |
| `search.go` | typed query、PIT + search_after、CandidateRef |
