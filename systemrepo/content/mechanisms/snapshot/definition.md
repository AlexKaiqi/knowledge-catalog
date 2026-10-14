---
entity: kc/mechanism/snapshot
aspect: definition
schema_ref: schema/system/explanation/v1
path_hint: mechanisms/snapshot/definition.md
---
# Snapshot

底层不可变版本、文件树与条件推进的协议边界。

Snapshot 不解释对象或查询，上层分别使用它发布对象和组合文件。介质必须提供固定版本与条件推进，能力不足应明确拒绝。

相关说明：[版本](../../concepts/version/definition.md)、[仓](../../concepts/repository/definition.md)、[Writer](../../interfaces/writer/definition.md)。
