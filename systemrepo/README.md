# SystemRepo

平台完整出版物存放在 `content/`；`publication.go` 聚合 Object 基础可信 Schema 与各能力的可信合同数据。`PublishSystem` 显式验证并经 Writer 发布，完整出版不进入 Object 根或基础 Writer。

对象身份、内容、版本与既有系统发布回执保持，系统初始化与调用时不隐式安装业务处理器。
