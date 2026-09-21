# Agent 须知

这是 **Knowledge Catalog 通用知识底座**：Catalog 协议的 **Go 参考实现**（身份、来源、写边界、Workspace 组合、维护闭环）。不是检索应用，也不是某个开源元数据产品的 fork。协议旅程用例在 `.data/scenes/`：组织、维护、执行、断言规范和场景不变量见 [`.data/scenes/README.md`](.data/scenes/README.md)。走查叶夹具（清河茶铺表/作业/语义/SQL）写在 `named-repositories-created` 的 `_materials/`，不要另起 `.data/data-warehouse/`，也不要让场景去读已删除的数仓目录。

## 工作方式（用户最新约定）

- 当前实际使用的是 lakeFS；Dolt 只是 adapter 抽象的验证对象，不主动扩展 Dolt 的验证与优化工作。
- 默认只做实现，测试由用户在其他环境执行。除非用户明确要求，不运行测试、验收、压测或其他自动验证，也不为验证启动服务、容器或安装依赖。
- 可以随实现补充必要的回归测试代码，但不自行执行。交付说明实现范围与尚未验证的部分，不把实现完成写成验收通过。
- 以下命令和验证流程仅在用户明确要求验证时适用。

## 命令

```bash
export PATH="$HOME/.local/go/bin:$PATH"   # 若系统 go 过旧
make check-docs                 # 文档图 OKF + 设计文档五段合同
make docs-serve                 # 本机 UTF-8 HTML 阅读 product.html 与设计 Markdown
make test                       # 已准备的 deploy-local lakeFS 测试栈上的场景
make test-contracts             # 显式旧组件/架构/应用合同组合（仍含 Dolt 夹具）
make test-all                   # 再跑 Gitea / Dolt / OpenSearch / Linux FUSE
go run ./cmd/kc -- help
```

局部 `go test` 只用于定位。协议代码是 Go 1.23+；Python 用 `.venv`。走查叶运行见 `.data/scenes/README.md`，不要在本文复制。

## 红线

- 不要在仓库根加 `collectors/`、`src/`、`tests/scenarios/`、源系统客户端或业务故事包。走查实体/Aspect/接入方 runtime 只放对应场景节点 `_materials/`；规模生成器只放 `.data/scale/`。
- 不要把 schema 写成项目源码。Schema 是知识对象，走 Writer；草稿只放 `.data/`。
- 不要把协议字段、错误码、状态机写进 `AGENTS.md` 或设计 Markdown。已选定形状在公开 Go API、包 README、Conformance；它们必须符合设计，不能反向收窄设计。
- 不要改 `docs/graph/` 以外的方式维护文档关系；不要给设计 Markdown 加会让正文被当成 YAML 解析的 frontmatter。
- 不要删除或削弱契约/conformance/架构守卫断言，不要用 skip 让测试变绿。
- 不要直写 git 绕过 Writer；不要新增 PATCH/跨 Repo 事务/APPEND Surface。
- 不要把知识协议写进 `catalog/`；不要把 live 资源伪装成 `snapshot.Store`。
- 不要安装未经批准的依赖；不要提交，除非用户明确要求。
- 改协议旅程用例前读 `.data/scenes/README.md`。不要只写 `command succeeds`；不要把 `cli/testdata/scenes` 当成协议场景树。
- 其它路径禁区、发权、hook/gate、dsh-plugin 运行时约束见拥有该主题的文档和包 README。

## 交付

1. 先读 `docs/graph/`，不要靠 Markdown 里的「参见」猜相关文档：
   - `documents/*.okf` 的 `ownerTopics` 决定打开哪一篇；
   - `relations/*.okf` 里 `from`/`to` 等于当前文档的边才是依赖/细化/验证关系。
2. **只打开本任务碰到的 owner 文档**，以及上一步扫到的直接相关篇。不要把 20 多份设计全文当默认上下文。
3. 涉及形状、命令、错误码：再读对应包 README、公开代码和 Conformance。它们是已选定合同，不能代替设计。
4. 有 `TASK.md` 时只认领一条 `[ ]`；契约（含 `make check-docs`）全绿且无 skip 才可 `[x]`。
5. 改行为前写出 Goal / Non-Goals（引用 owner）/ 不变量 ID / 选定与否决方案 / 接口指向设计合同或公开类型。不得用当前实现收窄设计。
6. 仅在用户明确要求验证时，先跑会失败的证据，再改代码，再跑到绿。文档变更后重读变更过的 owner 与 `docs/graph/`。主题所有权或文档间关系只改 `docs/graph/`，不改设计正文的文件头。

术语以 `docs/TERMINOLOGY.md` 为准。默认 ref 用 `snapshot.DefaultRef`。业务 `kc` 必须经 Server 与显式 principal。
