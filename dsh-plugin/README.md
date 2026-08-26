# dsh-loom

DeepSeek Harness (`dsh`) 的 Knowledge Catalog 集成。文件系统已不再由插件模拟：Linux 上先用 `kcfs` 把 Workspace 的多个成员目录挂到用户现有项目中，DSH、shell、IDE 和用户随后访问同一棵宿主文件树。

插件保留五项能力：

- 随包的 `knowledge-catalog` Skill：先回答 Catalog/Repository/Workspace/pin、
  写入边界、搜索缺能力和审计/历史/来源等用户疑问，再选择操作入口；
- 任务固定 pin 的 typed consumer tools：直接读、搜、浏览、Schema、关系和溯源；
- Operator 可调用的通用 `kc` 控制工具；
- 固定 Workspace pin 的 live resource 访问工具；
- 人用的 Catalog/Workspace 只读浏览页面。

## 共享宿主文件系统

Workspace 配方直接决定多个落点，不再有统一 `.knowledge` 包裹目录，也没有 `KC_MOUNT_PATH`：

```yaml
name: agent-workspace
mounts:
  - repository: kr://acme/team/docs
    selector: refs/heads/main
    path: docs/team
    subPath: handbook
  - repository: kr://acme/team/docs
    selector: refs/heads/main
    path: docs/runbooks
    subPath: runbooks
  - repository: kr://acme/org/policy
    selector: refs/heads/main
    path: knowledge/policy
    subPath: published
```

在现有项目中启动只读挂载：

```bash
go build -o kcfs ../cmd/kcfs

./kcfs plan \
  --home /var/lib/kc \
  --workspace agent-workspace \
  --root /path/to/my-project

./kcfs mount \
  --home /var/lib/kc \
  --workspace agent-workspace \
  --root /path/to/my-project
```

第二条命令前台运行到 `SIGINT` / `SIGTERM`。在另一个终端从同一项目启动 DSH：

```bash
cd /path/to/my-project
dsh --profile dsh-loom
```

此时用户、IDE、shell 和 Agent 看到相同内容。例如：

```bash
cat docs/team/README.md
rg incident knowledge/policy docs/team
git status
```

Agent 继续使用 DSH 原生 `read` / `list` / `glob` / `grep` / `rg` 和 shell；`cordis.patch.yml` 不再替换 `ctx.fs`，也不再关闭 stock search 工具。

项目本身可以是 Git 仓或普通目录。唯一的目录要求是每个精确 mountpoint 在挂载时不存在或为空；它的父目录和项目里的其它内容不受影响。宿主要求 Linux、`/dev/fuse` 和 `fusermount3`（通常由发行版 `fuse3` 包提供）。容器中还需把 `/dev/fuse` 和相应 mount capability 交给容器。

一次 `kcfs mount` 只 Resolve 一次 Workspace，所有成员固定为同一个 `pinId`，直到挂载进程退出。重新启动 `kcfs` 才会跟随 selector 的新位置。首版挂载只读；知识写入仍显式调用 `kc commit` / Writer，不能把 `close(2)` 冒充一次知识提交。

根 mount（`path: ""`）会隐藏已有项目根，因此 `kcfs` 明确拒绝。要附着到用户已有工作区，配方中的每一项都应声明非根目录。单文件注入也暂不支持；`subPath` 应指向目录树。

## 控制与身份

本地开发中，`KC_AS` / `X-Kc-As` 是显式 principal；空值代表本机 Owner。认证服务使用 `KC_AUTH_TOKEN`，两者不能同时设置。`kcfs --as` 同时检查 Workspace read grant 和每个 Repository 的 read grant；无权成员不会成为宿主 mountpoint。

`kc` 工具会丢弃模型传入的 `as`、`home`、`listen`，身份和本地运行坐标只能来自 DSH composition。每次调用都携带请求 ID。

```bash
export KC_SERVE=http://127.0.0.1:7380
export KC_CATALOG=kr://acme/catalog
export KC_WORKSPACE=agent-workspace
export KC_AS=consumer
dsh --profile dsh-loom
```

Agent 可以直接调用 `knowledge_read` 或 `knowledge_search`，不需要先执行初始化
工具。插件从当前目录或任一父目录的 Workspace binding，或者
`KC_CATALOG` / `KC_WORKSPACE` 取得消费范围，只 Resolve 一次；随后所有 typed
工具和 live resource 在同一 DSH Agent 任务内自动复用该 pin。任务结束时插件按
DSH 宿主生命周期释放本地上下文；KC 不创建 `WorkspaceSession`、`sessionId` 或
服务端 Session Store。`knowledge_context` 只用于查看身份、绑定来源和 pin。模型
不能改写 principal、onBehalfOf、Catalog、Workspace 或 pin。

对象 ID 未知时，优先 `knowledge_search`；无检索投影时使用宿主挂载中的 `rg`，
或用带 `objectPrefix` 的 `knowledge_list` 浏览 canonical ID。过滤字段必须来自
`knowledge_schema`，不能猜裸 path。已知对象的一跳关系用
`knowledge_relations`，来源证据用 `knowledge_provenance`。

`KC_SERVE` 仍服务控制工具和浏览器；它不承载 Agent 文件 I/O。`KC_AUTH_TOKEN` 是当前 DSH 用户的登录凭证，不要与 Snapshot adapter 使用的 `KC_GITEA_TOKEN` 混用。

## Live resource

Aspect 可内嵌 Binding，也可引用 ResourceDescriptor。`resource(object, aspect, operation, input)` 先在当前 Workspace pin 上解析声明，再调用 `KC_RESOURCE_ACCESS_URL/v1/access`。endpoint、token、cursor 和实时内容都留在墙外运行时；访问不会自动写入知识。

## 人用浏览器

Catalog 页面仍通过 `kc serve` 的 `vfs-list` / `vfs-read` 展示 Workspace、mount 元数据、文本预览和精确 commit。这个 HTTP 接缝只属于观察 UI，不再是 Agent 文件系统。刷新会重新 Resolve；浏览器不接收服务端凭证。

## 开发

```bash
cd dsh-plugin
npm install --legacy-peer-deps
npm run build
npm test

# 需要真实模型凭证；只加载 Skill，禁止借助 shell/文件系统猜答案
make -C .. test-agent-ux-e2e

dsh plugin --profile dsh-loom add link:$PWD
dsh --profile dsh-loom
```

主要文件：

```text
src/index.ts    package 激活入口；不提供 FileSystem
src/control.ts  kc Agent tool 与本地服务 bootstrap
src/context.ts  身份、Workspace、任务固定 pin 与宿主生命周期清理
src/knowledge.ts typed consumer tools 与面向 Agent 的错误引导
src/resource.ts live resource 工具
src/skill.ts    bundled Skills
src/client.ts   浏览器用的 kc serve VFS 客户端
src/web.ts      host 侧只读 HTTP 桥
src/browser.tsx Catalog 页面
```

`scripts/e2e_agent_questions.py` 用三组自然语言问题验证 Agent 的概念解释、接入建议
和故障恢复答案，并保存回答、tool trace 与语义 oracle；`scripts/e2e_agent_roles.py`
继续验证六个独立角色的真实操作闭环。

旧 Agent-only filesystem/search 及其 text/tree/error 辅助代码已经删除；`npm run build` 会先清空 `dist/`，避免把历史产物带入插件包。Linux FUSE 集成测试应在具备 `/dev/fuse` 的 CI/宿主运行；macOS 上可使用 `kcfs plan` 验证配方和 pin，但不能执行 mount。

仓库提供 Linux smoke test：`../scripts/e2e-kcfs-linux.sh`。它验证两个独立 mountpoint、同一 pin、宿主 `cat`/`rg`、只读拒绝、卸载清理和原项目文件不变。
