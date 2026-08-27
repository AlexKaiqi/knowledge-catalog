# internal/repofile/

② 知识单元的磁盘 codec，不是 Snapshot Store。`tree` 管内存索引，`codec` 管 frontmatter，`layout` 管路径，`assemble` 管 Aspect 拼装，`digest` 管声明摘要，`apply` 把 PUT/REMOVE 编译成字面树变化。

`*.okf` 是显式的知识单元扩展名；`*.aspect.yaml` 适合 Schema 草稿。两者都使用同一份 frontmatter（`object_id`、可选的 `aspect_name` / `member_key`、`kind`、`schema_ref`），正文可写 JSON 或结构化 YAML。`kc ingest --dir` 将它们机械编译为 ChangeSet，不引入领域转换。
