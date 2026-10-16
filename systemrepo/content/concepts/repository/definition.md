---
entity: kc/concept/repository
aspect: definition
schema_ref: schema/system/explanation/v1
path_hint: concepts/repository/definition.md
---
# 仓

按对象发布协议维护内容的 Repository，形成 Snapshot 版本图、历史、来源、写入和权限的边界。对象身份只由 object_id 确定；同一个对象的不同切面可以分布在多个仓，读取依据分别记录来源仓与版本。

Reader 在固定 commit 上解释对象声明。普通文件仓也能参与 Dataset 文件组合，但未按协议声明的文件不会自动成为对象。不同读者集合、写权威或发布节奏通常需要分仓。归档保留身份与历史。

相关说明：[Snapshot](../../mechanisms/snapshot/definition.md)、[对象](../object/definition.md)、[身份认证与授权](../../mechanisms/authorization/definition.md)、[登记空间](../registration/definition.md)。
