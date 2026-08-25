# internal/repofile/

② 知识单元的磁盘 codec，不是 Snapshot Store。`tree` 管内存索引，`codec` 管 frontmatter，`layout` 管路径，`assemble` 管 Aspect 拼装，`digest` 管声明摘要，`apply` 把 PUT/REMOVE 编译成字面树变化。
