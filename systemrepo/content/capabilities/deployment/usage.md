---
entity: kc/capability/deployment
aspect: usage
schema_ref: schema/system/capability-usage/v1
path_hint: capabilities/deployment/usage.md
---
# 部署与恢复：使用与核对

部署方在配置中明确绑定实际 system Snapshot；首次初始化将项目的 `systemrepo/content` 出版源经 Writer 发布到空仓并登记到 登记空间。系统发布只初始化空仓或核对相同出版物，已有不一致内容不得覆盖。Server 启动只恢复耐久状态并核对实际系统仓，运行读取实际仓；配置、权限账、系统出版物丢失或来源不可用时失败关闭，没有默认内置仓。部署状态不替代查询投影状态和调用权限。

选择 [调用入口](entrypoints.yaml) 后，先运行对应叶子 help 查看当前版本的操作数、参数和输入形状。系统仓保留任务语义，完整命令合同由 help 提供。
