---
entity: kc/guide/first-publication
aspect: guide
schema_ref: schema/system/usage-guide/v1
path_hint: guides/first-publication/guide.md
---
# 首次发布领域对象

1. 目标业务仓已存在，调用者有发布与读回权，已取得当前基点。
2. 阅读 Schema、对象和切面概念；读取系统基础 Schema，在目标仓发布所需同内容副本。
3. 准备领域 Schema 与匹配实例，为正文明确身份、来源和路径提示。声明与实例可同批发布。
4. 通过发布能力提交单仓变更，保留原 command-id 和接受回执。
5. 按回执 commit 读回声明、实例与来源。
6. 需要搜索时在 Schema 中明确字段访问声明，需要实时访问时按插件 Schema 发布实现声明；对象发布、插件安装与投影就绪分别检查。
6. 成功证据：同一已接受版本能解释实例。系统仓不可作为业务写目标；索引随后独立追上。

所需能力：[理解对象类型](../../capabilities/schemas/definition.md)、[发布与修订对象](../../capabilities/publish-object/definition.md)、[精确读取与复核](../../capabilities/read-object/definition.md)。
