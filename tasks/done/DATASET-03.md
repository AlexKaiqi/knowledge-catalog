# DATASET-03 · 逐文件交付贯通、dataset clone 与 Dataset 管理前端

- [x] 本轮完成：U10 逐文件交付贯通——`KnowledgeSetSource` 增 `File`+`Target`（交付寻址时 `Path` 必空）、发布校验目标唯一与祖先互斥（文件对文件、文件对前缀）、overlay 文件按目标替换并继承冻结坐标；File Gateway 交付寻址枚举/读取（`Path` 为根寻址判据、pin 重放保留已接受交付树、Length 0 默认 512 KiB）；C-21 收口（`datasetFSRepositoryPath` 归一化后必须仍落在所选子树内）；`kc dataset clone` 物化当前服务版交付树（非空目录拒绝、只随服务版演进、argv 旅程 `TestDatasetCloneJourney`）；`/ui/` Dataset 控制台进 httpsurface 闭集（87 条）并带页面证据测试；webui vendor（lakeFS v1.58.0 pinned，Apache-2.0，保留 LICENSE/NOTICE）构建产物嵌入 `cli/webapp`。文档：TEST_CATALOG C-17–C-21 转「已定位」并登记证据、reviewed/dataset.md U9/U10 与结尾约束、CLI_EVALUATION 增 clone 行并逐字核对 60 条 help 引用、client/README 补 File Gateway 节；「测试由用户在其他环境执行」表述经用户纠偏废弃（AGENTS.md/TASK.md）。验证：`go test ./cli/`（无环境与 `KC_TEST_OPENSEARCH_URL` 两遍口径）、`./catalog/ ./client/ ./home/ ./internal/... ./discovery/projection/`（带 OpenSearch）全绿，`make test` 96.9s PASS、`make test-contracts` 640.8s PASS（公开命令覆盖 59/59）、`make check-docs` PASS。

**Goal：** 发布、目录枚举、文件读取与授权对逐文件条目完整成立：发布产出 `DatasetItemFile` 条目并校验目标冲突，文件网关按交付目录树枚举/读取文件条目（支持改名与多来源混排进同一交付目录），相对路径任何位置的父目录跳转都被拒绝；`kc dataset clone` 把已发布 Dataset 的当前服务版交付目录树物化成本地普通目录（产品 argv 不收 `--pin`，见 `doc/design/affordance.md`），不依赖 kcfs。owner：[`doc/design/dataset.md`](../../doc/design/dataset.md)（U6/U9/U10、固定范围与生命周期）、[CLI 交互](../../doc/design/affordance.md)（产品 argv）、[`httpsurface`](../../httpsurface/README.md)（闭合路由表）。

**Non-Goals：** 不把 Dataset 做成可写目标，不复制字节进 Catalog，不给逐文件条目建独立宿主挂载点，不扩展跨仓事务，不在 clone 时绕过 Dataset 消费授权或写入清单外路径；本轮不改 kcfs 的 FUSE 能力边界（逐文件条目在 FUSE 计划中显式拒绝并明确报错，混合 eager 文件与 lazy 目录挂载的 datasetfs 扩展留作后续），不新增 HTTP 路由（clone 与逐文件交付复用既有闭合面，请求形状做向后兼容扩展）。

**不变量：** `KS-01`、`KS-02`、`V-01`、`AUTH-01`–`AUTH-03`。清单 item 发布时钉 commit；空清单不放行整仓；同一目标文件只有一个明确来源，冲突显式拒绝，不按挂载或来源顺序覆盖；逐文件条目与所属仓的其它条目共享同一冻结 commit；路径规范化后仍须处于选定子树内。

**选定与否决：** 选定逐文件条目搭乘现有 `KnowledgeSetSource` 形状（`File`+`Target` 字段，`Path` 必须为空），recipe `.kc-dataset.yaml` 与 overlay 增加 `files:` 段，发布时静态校验目标唯一性，内容级同名冲突在交付时显式报错（发布不解析来源树内容，不做发布期目录遍历）；网关请求在 `MountPath+Directory/File` 之外增加可选 `Path`（交付路径）以寻址挂载子树外的文件条目，文件条目只在首页合并进枚举结果。否决发布期遍历来源树做冲突检查（发布必须不依赖来源树读取）、否决按顺序静默覆盖、否决为 clone 新开 HTTP 路由、否决 clone 生成 manifest 文件（消费坐标由服务端发布记录承担，回执只打印版本与来源 commit）。

**接口与验收：** `catalog.KnowledgeSetSource` / `DatasetItem` / `HashResolved` / recipe / overlay；`cli` 文件网关（`dataset_file_service.go`、`datasetfs.go`）与 `client` typed 请求；`kc dataset clone` 进 `cli/surface.go` 公开闭集（动作 `file.read`×Dataset 消费）；webui vendor 保留上游 LICENSE/NOTICE 并记录 pinned commit，API 层只接 httpsurface 闭合路由。回归：U10 发布/枚举/读取/授权端到端、目标冲突发布拒绝、C-21 断言转绿、clone 物化与拒绝非空目录、clone 只随当前服务版演进。代码修改必须通过测试；契约全绿且无 skip 才勾选。
