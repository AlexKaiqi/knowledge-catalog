# OBJECT-GUIDELINE-01 · 对象模块重构指南（2026-10-12）

- [x] 本轮完成文档整理：保存可重复查看的目录、职责、依赖、迁移与验收 guideline，将图示资源纳入仓库并登记文档图；文档及质量门禁通过，不代表代码迁移或业务验收完成。
- Goal / owner：目标与迁移边界归 `doc/design/object.md`；能力组织、架构与请求责任分别沿 capability-organization / core-architecture / service，发布发现语义沿 object-system 及其直接 owner。
- Non-Goals：不改源码、公开协议、system 出版物或场景树，不新增业务对象发布门槛，不改 Snapshot 在途实现，不安装依赖、不启动服务、不提交；保留其它在途改动。
- 不变量 / 选定：沿用 A-01 / L-01、I-01 / V-01、API-01、CA-01、R-01 / R-02、OI-01～05；选定同级 object / interfaces / discovery / liveaccess 与公开一级入口，基本读写归 Object，任务服务归所属能力；否决 objectapp 兜底与宿主执行请求规则。
- 验证：本轮运行 `make check-docs`、`make quality`，检查 Markdown 与图示资源；这些证据只验文档及工程门禁，不代表目标结构、业务用例或真实部署通过。
- 验证结果：`make check-docs` 为 31 文档 / 136 关系通过；`make quality` 退出码 0，格式、tidy、vet、staticcheck、复杂度、体积、克隆及耦合门禁通过。两张 PNG 完整性与两份 SVG XML 检查通过，图示视觉复核通过；`git diff --check` 通过。未运行业务测试或真实部署，未提交。
- 延后问题的处置：ObjectID 空白校验（`object/address.go` / Writer）随基本读写迁移修正；显式 Schema pin 与索引采用依据不一致（`reader/schema.go` / `discovery/projection/spec.go` / `discovery/projection/extract.go`）随发现拆分补同依据回归；只读 Repository 暴露完整 Store 写能力（`object/repository.go`）随读写端口拆分收窄。位置、信号、判定及触发条件详见指南，均未在本轮修复。
