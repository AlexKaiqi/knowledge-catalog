---
entity: kc/capability/schemas
aspect: usage
schema_ref: schema/system/capability-usage/v1
path_hint: capabilities/schemas/usage.md
---
# 理解对象类型：使用与核对

先判断已有 Schema 是否适合资料。可以从 `kc show` 选择公开 Dataset，经 `kc dataset show` 记录 revision，再沿 `kc files list/read` 取得对象及其引用的 Schema 原文件；列目录与读取沿用同一 revision。已有来源仓读取权时，也可用 `kc schema list` 浏览类型，再查看具体声明。名单是对象类型入口，不是对象实例目录，也不是合同正文。

`kc schema describe` 返回供理解结构的规范化展示，包含字段 path/type 和 `fields` 数组。它的 `definition` 或整个响应不能直接作为 Schema 输入；原始 Schema 的 `fields` 是按字段名组织的对象映射。要复用原值，使用 `kc read` 按 Schema 对象身份与固定 commit 读取，取返回的 `value`；要复用完整文件，使用 Dataset 的 `kc files read`。不要把 JSON 响应信封写成正文。

实例与 Schema 使用明确的版本依据，引用在目标发布中必须可解析。可将原始 Schema 文件与实例同批提交；
Meta Schema 与索引声明 Schema 的副本须保持与出版物一致，其余类型按普通兼容规则演进，具体范围见
[Schema](../../concepts/schema/definition.md)。Schema 只描述结构，查询能力与外部取值分别由索引声明和取值描述说明。
完整复用步骤见 [首次发布领域对象](../../guides/first-publication/guide.md)。

选择 [调用入口](entrypoints.yaml) 后，先运行对应叶子 help 查看当前版本的操作数、参数和输入形状。系统仓保留任务语义，完整命令合同由 help 提供。
