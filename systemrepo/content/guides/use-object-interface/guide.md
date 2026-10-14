---
entity: kc/guide/use-object-interface
aspect: guide
schema_ref: schema/system/usage-guide/v1
path_hint: guides/use-object-interface/guide.md
---
# 使用对象能力

1. 按允许范围及固定版本精确读取对象的 Interface 实现切面。
2. 查看能力合同和实现 Schema，选择实现成员；多个实现不能靠文件顺序决定。
3. 从 [调用入口](../../capabilities/object-interfaces/entrypoints.yaml) 查看当前版本 help，按其合同提交操作和输入。
4. 保存声明依据与实际结果依据。LiveAccess 当前状态附观察依据，声明版本不能代替观察版本。
5. 如需形成说明或结论，显式通过 Writer 发布并记录真实输入依赖和算法。

缺合同、处理器未装配、授权拒绝和运行故障分别处理，不能补读范围外文件、换成新版或把故障解释为空值。历史 ResourceDescriptor 调用沿既有兼容入口执行。

成功证据：返回对象、实现和合同的固定依据，以及该能力所要求的实际结果依据。

相关说明：[对象能力分派](../../mechanisms/object-interface-dispatch/definition.md)、[动态资源值](../use-dynamic-values/guide.md)、[修订切面](../revise-an-aspect/guide.md)。
