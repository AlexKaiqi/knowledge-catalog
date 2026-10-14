---
entity: kc/concept/version
aspect: definition
schema_ref: schema/system/explanation/v1
path_hint: concepts/version/definition.md
---
# 版本

仓的 commit 固定已发布内容，Dataset 的发布记录固定文件范围及各来源 commit。这两种版本分别回答仓中保存了什么、消费者采用了什么。

一次请求使用开始时固定的依据。动态值另带观察依据，不能把观察当成仓提交，也不能由固定声明推导动态值可重复读取。对象内容的修订或回退通过新提交保留历史。系统出版物由部署写入实际仓，commit 固定完整内容；启动核对当前版本，运行读取实际仓。

相关说明：[Snapshot](../../mechanisms/snapshot/definition.md)、[固定消费依据](../pin/definition.md)、[动态资源访问](../../mechanisms/dynamic-access/definition.md)、[来源](../provenance/definition.md)。
