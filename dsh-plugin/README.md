# dsh-loom

DeepSeek Harness (`dsh`) 的 Knowledge Catalog 集成。文件系统已不再由插件模拟：Linux 上先用 `kcfs` 把 Workspace 的多个成员目录挂到用户现有项目中，DSH、shell、IDE 和用户随后访问同一棵宿主文件树。

插件保留四项能力：

- 随包的 `knowledge-catalog` Skill；
- Agent 可调用的 `kc` 控制工具；
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

一次 `kcfs mount` 只 Resolve 一次 Workspace，所有成员固定为同一个 `pinId`，直到会话结束。重新启动 `kcfs` 才会跟随 selector 的新位置。首版挂载只读；知识写入仍显式调用 `kc commit` / Writer，不能把 `close(2)` 冒充一次知识提交。

根 mount（`path: ""`）会隐藏已有项目根，因此 `kcfs` 明确拒绝。要附着到用户已有工作区，配方中的每一项都应声明非根目录。单文件注入也暂不支持；`subPath` 应指向目录树。

## 控制与身份

本地开发中，`KC_AS` / `X-Kc-As` 是显式 principal；空值代表本机 Owner。认证服务使用 `KC_AUTH_TOKEN`，两者不能同时设置。`kcfs --as` 同时检查 Workspace read grant 和每个 Repository 的 read grant；无权成员不会成为宿主 mountpoint。

`kc` 工具会丢弃模型传入的 `as`、`home`、`listen`，身份和本地运行坐标只能来自 DSH composition。每次调用都携带请求 ID。

```bash
export KC_SERVE=http://127.0.0.1:7380
export KC_WORKSPACE=agent-workspace
export KC_AS=consumer
dsh --profile dsh-loom
```

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

dsh plugin --profile dsh-loom add link:$PWD
dsh --profile dsh-loom
```

主要文件：

```text
src/index.ts    package 激活入口；不提供 FileSystem
src/control.ts  kc Agent tool 与本地服务 bootstrap
src/resource.ts live resource 工具
src/skill.ts    bundled Skills
src/client.ts   浏览器用的 kc serve VFS 客户端
src/web.ts      host 侧只读 HTTP 桥
src/browser.tsx Catalog 页面
```

旧 `fs.ts` / `search.ts` 只为过渡期源码测试保留，已从 package exports 和默认 composition 移除，不属于运行路径。Linux FUSE 集成测试应在具备 `/dev/fuse` 的 CI/宿主运行；macOS 上可使用 `kcfs plan` 验证配方和 pin，但不能执行 mount。

仓库提供 Linux smoke test：`../scripts/e2e-kcfs-linux.sh`。它验证两个独立 mountpoint、同一 pin、宿主 `cat`/`rg`、只读拒绝、卸载清理和原项目文件不变。
