---
entity: kc/capability/publish-knowledge
aspect: usage
schema_ref: schema/system/capability-usage/v1
path_hint: capabilities/publish-knowledge/usage.md
---
# 发布与修订知识：使用与核对

读取当前基点，准备声明与实例或一个切面修改；必要时 diff，随后提交。保留 command-id 和回执，以接受的版本读回。响应丢失用原身份查询回执；基点冲突重读核对，不能关闭保护。

选择 [调用入口](entrypoints.yaml) 后，先运行对应叶子 help 查看当前版本的操作数、参数和输入形状。系统仓保留任务语义，完整命令合同由 help 提供。
