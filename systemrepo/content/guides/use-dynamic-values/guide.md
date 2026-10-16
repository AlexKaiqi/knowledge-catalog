---
entity: kc/guide/use-dynamic-values
aspect: guide
schema_ref: schema/system/usage-guide/v1
path_hint: guides/use-dynamic-values/guide.md
---
# 使用动态资源值

1. 确定获准的对象范围和版本，读取 `kc plugin liveaccess help`，核对方面或成员上的取值来源及其 ResourceDescriptor。
2. 取值来源只引用取值描述；来源 origin、模式与身份要求写在描述上，Schema 只约束值结构。调用者满足 KC 与源侧访问要求。
3. 按对象 URN 与定位符读取所需单元；插件透明取值，每个外部单元的观察依据与值并列返回。
4. 核对逐值失败、观察范围、时效与源准入；固定声明版本不会冻结外部当前值。需要保存为对象时显式选择并经 Writer 发布到业务仓。
5. 成功证据：值符合所采用的 Schema，观察依据可核对。源拒绝不回退缓存，观察不被伪装成仓提交。共享索引只接受描述为 shared 的外部单元；per-caller 单元只按实际调用者取值。

所需能力：[理解对象类型](../../capabilities/schemas/definition.md)、[访问动态资源](../../capabilities/resource-access/definition.md)、[发布与修订对象](../../capabilities/publish-object/definition.md)。
