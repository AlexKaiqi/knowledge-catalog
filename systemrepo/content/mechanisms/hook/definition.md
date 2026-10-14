---
entity: kc/mechanism/hook
aspect: definition
schema_ref: schema/system/explanation/v1
path_hint: mechanisms/hook/definition.md
---
# 出站扩展

在明确配置的应用动作前后调用外部脚本或 HTTP 的出站扩展机制。

授权先成立，Hook 再按动作与时机匹配配置，不修改变更内容。前置失败可阻止操作；已接受动作后的投递失败应恢复投递，不撤销已接受的结果。读取、搜索和交付不挂出站 Hook，内部版本推进通知也不与它共享配置或交付承诺。

相关说明：[Writer](../../interfaces/writer/definition.md)、[变更评审与合并](../governance/definition.md)。
