# declarativeindex/internal/testfixture

声明式索引模块测试共用的 Writer 材料与物理引擎夹具。只消费通用 Discovery 端口、Object/Testkit 与
真实公开 OpenSearch adapter，不导入 Index 或 Controller；调用方自行构造被验收的真实对象。

`StateEngine` 保留编译文档、Meta、流式 publication 与分析补判的原测试 oracle；`StateLookup`
记录实际 lookup 次数。`StateProjection`、`CommittedSearchablePolicy`、关系和字段材料均经 Writer
提交，供查询与控制测试消费同一份固定来源。`LiveEngineOpener` 仅绑定已有
`KC_TEST_OPENSEARCH_URL`，缺配置明确失败，不启动服务。

`SnapshotCheckpoint` 经集群快照仓库保存可复用的索引状态，只捕获本调用命名空间前缀下的索引。恢复时
先删除目标前缀下的精确索引名并确认已清空，再按 `rename_pattern` 恢复到另一个命名空间，最后回读
文档数与捕获值比对。引擎对已存在的索引名整请求失败，不提供覆盖或跳过语义，因此删除是前置条件而不是
优化。`Release` 按记录的前缀回收捕获索引；集群缺 `path.repo` 或仓库未注册时构造即失败。

`SceneFingerprint` 的覆盖范围是测出来的，不是估的：只哈希决定索引内容的输入（`_meta.yaml`、
`_bundles.yaml`、`_materials/` 材料、`.feature` 探针、声明文件），排除 README、字幕、图片与
`_results`/`_build`/`__pycache__` 生成物。按节点全文件或整仓哈希都会让一次文档改动作废所有副本，
而 `scripts/validation.py` 的整仓摘要正属于后者，故不复用。

`Checkpoint.Sources` 记录被捕获文档上打戳的 `(source_repository, source_commit)` 集合。这里的 commit
是投影对象被读取时所在仓库的 commit，不是索引 basis：`response.go:84` 把命中文档的 `source_commit`
直接当作候选 basis 返回，所以一条指向已不存在来源的文档会产出指向不可读数据的候选，而文档计数校验
仍然全部通过。lakeFS 实测：commit 哈希了创建时间戳与父提交，同内容重建不可复现；仓库删除重建后旧
commit 不再解析（换新 storage namespace 得 410，复用原 namespace 被 lakeFS 拒绝，报
`storage namespace already in use`）。因此恢复必须比对来源身份，不一致就重建。Dataset 投影本就跨
来源仓库，所以多个来源是正常的，全部记录；一个来源身份都没有的捕获直接失败。

`Restore` 的 `sources` 参数由调用方传入本次运行的实际来源，顺序不影响相等判断，子集视为不同捕获。
