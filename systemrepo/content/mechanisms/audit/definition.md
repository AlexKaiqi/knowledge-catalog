---
entity: kc/mechanism/audit
aspect: definition
schema_ref: schema/system/explanation/v1
path_hint: mechanisms/audit/definition.md
---
# 访问审计

记录实际调用主体、访问动作、所用知识或文件版本，以及授权决定和访问结果。

访问证据保存在独立证据库中，有自己的保留和审计读取合同，不进入知识仓、登记空间 登记或搜索投影，也不能替代权限检查。用户接受或纠正结果另由反馈记录关联已有访问；反馈的用法见 [审计与反馈](../../capabilities/evidence/usage.md)。

相关说明：[来源](../../concepts/provenance/definition.md)、[身份认证与授权](../authorization/definition.md)、[版本](../../concepts/version/definition.md)。
