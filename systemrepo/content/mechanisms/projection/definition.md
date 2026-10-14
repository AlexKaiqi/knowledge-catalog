---
entity: kc/mechanism/projection
aspect: definition
schema_ref: schema/system/explanation/v1
path_hint: mechanisms/projection/definition.md
---
# 派生投影

从固定知识版本或动态观察构建的可重建查询状态。

Schema 同时承载结构与字段访问意图，按固定版本编译逻辑访问合同；结构校验与查询合同分别处理，物理实现与就绪分别判断。发布知识不等待索引成功。检索取得同依据候选后回读正文；无满足合同的投影时明确失败，消费请求不扫描全仓或同步建索引。

相关说明：[声明式索引](../../concepts/index-declaration/definition.md)、[版本](../../concepts/version/definition.md)、[动态资源访问](../dynamic-access/definition.md)、[Schema](../../concepts/schema/definition.md)。
