# retrieval/elasticsearch/

Elasticsearch managed Projection adapter；不是权威 Store，也不保存可直接返回给消费者的知识正文。

| 文件 | 负责 |
|---|---|
| `config.go` | 非密配置、环境凭证策略、Engine opener |
| `client.go` | HTTP transport、认证头和 index mapping |
| `projection.go` | Meta、Rebuild/Apply/Count 与物理文档编码 |
| `search.go` | capability Probe、continuation、Search DSL 翻译、CandidateRef |
