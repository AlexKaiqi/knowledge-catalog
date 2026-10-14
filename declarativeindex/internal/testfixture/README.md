# declarativeindex/internal/testfixture

声明式索引模块测试共用的 Writer 材料与物理引擎夹具。只消费通用 Discovery 端口、Object/Testkit 与
真实公开 OpenSearch adapter，不导入 Index 或 Controller；调用方自行构造被验收的真实对象。

`StateEngine` 保留编译文档、Meta、流式 publication 与分析补判的原测试 oracle；`StateLookup`
记录实际 lookup 次数。`StateProjection`、`CommittedSearchablePolicy`、关系和字段材料均经 Writer
提交，供查询与控制测试消费同一份固定来源。`LiveEngineOpener` 仅绑定已有
`KC_TEST_OPENSEARCH_URL`，缺配置明确失败，不启动服务。
