---
entity: kc/concept/system-repository
aspect: definition
schema_ref: schema/system/explanation/v1
path_hint: concepts/system-repository/definition.md
---
# 系统知识仓

Knowledge Catalog 关于自身的版本化出版物，固定身份为 kr://kc/system，发布基础 Schema、模型、操作入口、运行机制、能力说明、使用指南和导航关系。

项目的 `systemrepo/content` 目录是出版源；平台部署时经 Writer 发布到明确绑定的实际仓，启动核对，运行时只读实际仓。缺失或不匹配时拒绝启动，没有默认内置仓。默认发现和读取来自明确授权声明。业务 Dataset 显式选择采用其中的文件。当前部署是否启用、就绪或授权，须查询实际服务。

相关说明：[Meta Schema](../meta-schema/definition.md)、[Dataset](../dataset/definition.md)、[身份认证与授权](../../mechanisms/authorization/definition.md)。
