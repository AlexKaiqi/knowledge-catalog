---
entity: kc/interface/cli
aspect: definition
schema_ref: schema/system/explanation/v1
path_hint: interfaces/cli/definition.md
---
# CLI

供人、脚本与 Agent 使用的 kc 分组命令入口。业务操作调用同一 Server 应用能力。

系统仓解释已有概念、操作用途和使用方法，根 help 说明能做什么，组 help 列动作，叶子 help 给精确用法。入口切面从 CLI 登记生成，完整参数以同版本 help 为准。

相关说明：[身份认证与授权](../../mechanisms/authorization/definition.md)。
