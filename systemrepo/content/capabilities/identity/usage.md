---
entity: kc/capability/identity
aspect: usage
schema_ref: schema/system/capability-usage/v1
path_hint: capabilities/identity/usage.md
---
# 身份与准入：使用与核对

准备客户端服务配置，先登录并核对 whoami；准入状态回答当前能否进入。登录不恢复已撤销授权，服务地址变化时按目标服务重新认证。

选择 [调用入口](entrypoints.yaml) 后，先运行对应叶子 help 查看当前版本的操作数、参数和输入形状。系统仓保留任务语义，完整命令合同由 help 提供。
