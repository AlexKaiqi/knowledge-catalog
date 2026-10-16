# Object 文件格式

② 对象单元的磁盘 codec，不是 Snapshot Store。`tree` 管内存索引，`codec` 管 frontmatter，`layout` 管路径，`assemble` 调用根 Object 的唯一组装代数，`digest` 管声明摘要。`ApplyOperations` 整批调用 `object.ApplyUnits`，校验最终表示后只转换文件位置、序列化和字面树变化；`Apply` 是相同语义的单操作入口。

`ToObjectUnits` 是文件单元到逻辑维护单元的唯一字段转换，供格式层、Reader 的完整或所选单位读取及 Writer 的 Schema 规划共用。它保留声明、来源、内容与摘要，实际 `Path` 留在格式层；转换不添加另一份校验或组装规则。

每对象 locator 是可重建的派生读取元数据，保存维护 Address、实际 Path 与完整采用的 SchemaRef，不改变 Canonical 内容或对象身份。SchemaRef 保留显式历史 commit；已知空引用与旧元数据缺少引用证明分别编码。Writer 同提交更新，显式 `RebuildTreeLocators` 也生成同一形状。

`ReadObjectUnitLocations` 只读固定版本元数据，不为补坐标或 Schema 引用读取对象正文；旧 paths-only locator 仍可由 `ReadObjectLocator` 支持完整固定读与既有维护入口，但精确选取明确报告缺少能力。已有坐标但缺 Schema 引用证明时，元数据类型解释同样须显式重建；不能自动迁移固定历史版本。Canonical frontmatter 没有新增字段。

`LocatorSchemaIndexPath` 固定全部 Schema 的身份目录；`LocatorBindingIndexPath` 列出全部 binding 单元及其采纳的 Schema 与引用的取值描述（`BindingCoordinate`，`BindingSchemaObjectIDs` 与 `BindingReferences` 由其导出；旧目录缺 `Descriptor` 时 `BindingReferences` 报缺能力，须显式重建）。Writer 在同一 commit 按涉及对象维护两份目录；binding 分类沿单元自身的 `valueSource`，不要求消费方扫描 Schema 或单元正文来补分类；缺目录须显式重建。

`ReadSchemaReferrers` 从每 Schema 的派生条目返回固定版本引用者 Address；完整性标记使不存在条目可明确表示零引用。它不扫描仓、不读取 Schema 或实例正文来发现引用；旧完整 reverse metadata 可读，缺少证明须显式 `RebuildTreeLocators`。`ReadAddressUnit` 沿单元坐标只读一个维护单元，供 Schema 校验精确取得引用实例。

实例草稿用 `*.yaml`；`*.aspect.yaml` 适合 Schema 草稿。YAML/JSON 使用 frontmatter `object_id`、可选的 `aspect_name` / `member_key`、`kind`、`schema_ref`，正文可写 JSON 或结构化 YAML。Markdown 对象单元（仓根 `README.md`）用 `entity` / `aspect` 承载 Address，正文是 markdown，不是 JSON 信封。`kc writer commit --repo <仓> --command-id <命令身份> --dir <草稿目录>` 机械编译并发布这些文件，不引入领域转换；可先用 `kc diff --repo <仓> --dir <草稿目录>` 查看对照。已入库的 `*.okf` 仍可读取。
