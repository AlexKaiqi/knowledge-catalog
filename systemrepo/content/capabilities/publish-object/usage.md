---
entity: kc/capability/publish-object
aspect: usage
schema_ref: schema/system/capability-usage/v1
path_hint: capabilities/publish-object/usage.md
---
# 发布与修订对象：使用与核对

先选自己的目标仓，已有仓直接使用。首次整理资料时，从 `kc show` 选择公开 Dataset，用 `kc dataset show` 固定 revision，再用 `kc files list/read` 取得已有 Schema 和对象原文件；完整旅程见 [首次发布领域对象](../../guides/first-publication/guide.md)。复用适合的类型与格式，再确定自己的对象身份、正文和来源。

一个完整 JSON 值用 `kc writer put`；多个带文件头的对象文件，或 Schema 与实例一起发布，用 `kc writer commit --dir`。`put --file` 读取纯 JSON 值，不读取 frontmatter 文件或整个读取响应。目录文件的完整通用样例与规则由 `kc help writer commit` 提供，`kc diff --dir` 采用同一文件合同。

读取当前基点，准备完整单元；必要时 diff，随后提交。保留 command-id 和回执，以接受的版本显式限定目标仓读回。响应丢失用原身份查询回执；基点冲突重读核对，不能关闭保护。需要评审时开工作分支写入，评审后按审阅的分支 commit 合并；分支或目标移动后重新评审。

选择 [调用入口](entrypoints.yaml) 后，先运行对应叶子 help 查看当前版本的操作数、参数和输入形状。系统仓保留任务语义，完整命令合同由 help 提供。
