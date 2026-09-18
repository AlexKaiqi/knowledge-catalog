# internal/repofile/

② 知识单元的磁盘 codec，不是 Snapshot Store。`tree` 管内存索引，`codec` 管 frontmatter，`layout` 管路径，`assemble` 管 Aspect 拼装，`digest` 管声明摘要，`apply` 把 PUT/REMOVE 编译成字面树变化。

实例草稿用 `*.yaml`；`*.aspect.yaml` 适合 Schema 草稿。YAML/JSON 使用 frontmatter `object_id`、可选的 `aspect_name` / `member_key`、`kind`、`schema_ref`，正文可写 JSON 或结构化 YAML。Markdown 知识单元（仓根 `README.md`）用 `entity` / `aspect` 承载 Address，正文是 markdown，不是 JSON 信封。`kc ingest --dir` 将它们机械编译为 ChangeSet，不引入领域转换。已入库的 `*.okf` 仍可读取。
