---
entity: kc/mechanism/projection
aspect: definition
schema_ref: schema/system/explanation/v1
path_hint: mechanisms/projection/definition.md
---
# 派生投影

从固定对象版本或动态观察构建的可重建查询状态。

Schema 只描述结构，选进 Dataset 的索引成员描述构建与检索用途。按 Dataset 修订冻结来源版本、Schema、声明和能力清单，准备该修订的检索空间；声明、所指 Schema 或清单变化时重新核对受影响的成员。发布对象与发布 Dataset 是两个动作：前者不等待索引成功，后者须完成准备才接受新修订，失败保留当前发布版。

检索取得候选后按同一修订与来源版本回读正文。无满足合同的索引时明确失败，消费请求不扫描全仓或同步建索引。外部值只有 shared 的能进索引，命中回读取得新观察后按查询条件补判；只有当前修订刷新外部值，保留的旧修订沿用原观察。

相关说明：[声明式索引](../../concepts/index-declaration/definition.md)、[版本](../../concepts/version/definition.md)、[动态资源访问](../dynamic-access/definition.md)、[Schema](../../concepts/schema/definition.md)。
