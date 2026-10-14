---
entity: kc/concept/repository
aspect: definition
schema_ref: schema/system/explanation/v1
path_hint: concepts/repository/definition.md
---
# 知识仓

按知识发布协议维护内容的 Repository，形成知识身份、Snapshot 版本图、历史、来源与维护权威的边界。

Reader 在固定 commit 上解释知识声明。普通文件仓也能参与 Dataset 文件组合，但未按协议声明的文件不会自动成为知识对象。不同读者集合、写权威或发布节奏通常需要分仓。归档保留身份与历史。

相关说明：[Snapshot](../../mechanisms/snapshot/definition.md)、[对象](../object/definition.md)、[身份认证与授权](../../mechanisms/authorization/definition.md)、[登记空间](../registration/definition.md)。
