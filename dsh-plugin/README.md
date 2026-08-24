# dsh-loom

DeepSeek Harness (`dsh`) 的 Agent-first Knowledge Catalog 插件。它同时提供：

- 随包安装、在空 Workspace 之前即可发现的 `knowledge-catalog` Skill；
- Agent 可直接调用的 `kc` 工具，复用 Go CLI 的同一张 HTTP 动词表；
- Agent 可直接调用的 `resource` 工具，按当前 Workspace 中固定版本的 ResourceDescriptor 访问 live 资源；
- 把已解析 Workspace 暴露成文件树的 `ctx.fs` 与 `glob` / `grep` 工具；
- 给人查看同一棵树的原生 `Catalog` 页面。

启动 Agent 前，用户先在左侧选择已有 Catalog Workspace，或新建第一个 Catalog + 根 Repo + Workspace。随后 DSH 为该知识 Workspace 建立独立的会话锚点；Agent、VFS、搜索和 `kc` 工具从 Session 的 `cwd` 读取同一份绑定。Agent 进入后可以继续配置 Repo/权限与门禁、写入或发起 Proposal、验证合并、读取检索、审计与追溯。

agent 的 `read` / `write` / `edit` / `list` 走 Loom 的虚拟树（`kc serve` 的 `vfs-read` / `vfs-list` / `vfs-write`），按路径路由到各自的仓，**磁盘上不会检出一棵 checkout**。这和 `kc checkout`（真 git worktree）是两条路。

接缝对齐 [`@deepseek-ai/dsh-fs`](https://deepseek-harness.github.io/deepseek-harness/en/reference/subsystems/filesystem) 的 `FileSystem`，以及 [`docs/COMPOSITION.md`](../docs/COMPOSITION.md) §2.3.1。

## 从空目录开始

插件安装到 DSH profile 后，业务任务不要求用户预建 Catalog，也不要求另开终端启动服务：

```bash
mkdir empty-workspace && cd empty-workspace
export KC_HOME="$PWD/.kc-home"
# 可选：只作为新建表单的建议值，不再静默绑定 Agent。
export KC_WORKSPACE=agent

# 空 KC_AS 是本地 Workspace Owner；有角色时必须显式固定，例如 producer。
export KC_AS=
dsh --profile dsh-loom
```

首次调用 `kc` 工具时，插件会在 loopback 上惰性启动 `kc serve`。`KC_BIN` 可指定现成二进制；否则随包脚本依次使用 PATH 中的 `kc`、本地 Knowledge Catalog 源码，最后从上游源码构建缓存。远程 `KC_SERVE` 不会被插件自动启动。

首屏提供“新建并进入”或已有 Workspace 选择器。新建会执行 `init → repo-add → define-workspace → resolve`；进入后才创建绑定该 Workspace 的 Agent Session。每个绑定使用独立宿主锚点目录，因此切换 Workspace 会进入另一个 DSH Workspace/Session，旧会话不会漂移。Agent 仍通过 DSH Skill registry 自动获得 `knowledge-catalog` 操作规范，不要求用户手工加载 Skill。

## 身份与角色边界

插件支持两种互斥身份模式：

- 本地开发：`KC_AS` / `X-Kc-As` 是不可信的 principal 声明，用于验证授权语义；空值是本机 Owner。
- 真实认证：连接以 `--auth gitea` 启动的 `kc serve`，设置 `KC_AUTH_TOKEN`。插件发送 `Authorization: Bearer ...`，服务经 Gitea `/api/v1/user` 得到 `gitea:<numeric-id>`，不再发送 `X-Kc-As`。

`kc` 工具会丢弃模型传入的 `as`、`home`、`listen`，因此模型不能静默移除身份或改成本机 Owner。`KC_AS` 与 `KC_AUTH_TOKEN` 同时出现会直接拒绝启动。每次调用同时携带 `X-Kc-Request-Id`。

本地模式建议为 Catalog Owner、Producer、Reviewer/Gatekeeper、Consumer、Auditor 和 Unauthorized Actor 分别启动独立 session/composition。认证模式下可以使用同一个 profile，由每个 session 的真实 Gitea PAT 决定主体；KC rule 决定它能做什么。

## Agent 看见什么

例如 alice 的 notes workspace：

```text
notes/review.md                   ← kr://example/personals/alice
refs/policies/incident.md         ← kr://example/org/policies
```

agent 只看到这一棵树，不传 `--repo`。落点由 mount 路径决定。无权的仓经 `X-Kc-As` / `kc allow` 裁剪。

## 访问 live 资源

ResourceDescriptor 是知识树中的普通文件，例如 `resource/traces/payment-api`。Agent 把它的 `object_id` 交给 `resource` 工具，并选择文件中声明的 `status`、`window` 或 `lookup` 操作：

```text
resource(descriptor, operation, input)
→ 从当前 Workspace pin 读取 Descriptor
→ 调用 KC_RESOURCE_ACCESS_URL/v1/access
→ 返回 live 结果
```

Descriptor 只声明资源语义、runtime、协议和操作，不保存 endpoint 或 token。工具从 DSH composition 固定 principal、session、Agent preset 和请求 ID，同时传递 Descriptor 的 Repository/commit；模型不能覆盖这些字段。平台访问服务据此做授权并记录可审计 trace。

启动提供访问服务的 profile 时设置：

```bash
export KC_RESOURCE_ACCESS_URL=http://127.0.0.1:7480
```

不设置时知识读取仍可用，但 `resource` 工具会明确报告运行服务未配置。访问 live 资源不会自动写知识；需要沉淀时仍由 Collector 经 Writer COMMIT/APPEND 完成。

## 人如何查看 VFS

启动 web profile 后，Catalog 区先展示“新建 / 选择 Workspace”入口。进入后，Catalog 目录直接出现在 DSH 左侧现有的 Workspace/Session 导航区下方。点击目录中的文件会切换到主内容区顶部与“对话”并列的 **Catalog** 页签，并在右侧展开内容；Catalog 页本身不再重复目录，也不打开弹窗。导航区提供：

- Workspace 文件树和路径过滤；
- mount 目录名旁直接标注 Repository（悬停查看 selector、subPath 与本次解析的 commit），空 mount 也会显示；
- 文本预览（大文件最多预览 512 KiB，二进制只显示元数据）；
- 每次读取对应的 Repository 与 commit；
- **刷新**：重新 Resolve Workspace，同时重读当前选中文件，不沿用旧 pin。

这是观察面，不提供直接写按钮；编辑仍由 Agent 的 Workspace 文件工具或受治理的 `kc` 写路径完成。浏览器只访问 DSH 的同源只读桥，`KC_AUTH_TOKEN` 留在 host 进程中，不会下发到页面。文件列表也经过当前身份的 Workspace 权限裁剪。

## 连接已有服务（可选）

若已有独立运行的 Knowledge Catalog，可显式连接：

```bash
export PATH="$HOME/.local/go/bin:$PATH"
go build -o kc ./cmd/kc
./kc --home /tmp/kc-demo init --catalog kr://acme/catalog
./kc --home /tmp/kc-demo mount kr://acme/personals/alice
./kc --home /tmp/kc-demo define-workspace --workspace notes --revision 1 \
  --source kr://acme/personals/alice=refs/heads/main@
./kc --home /tmp/kc-demo serve --home /tmp/kc-demo
```

连接真实 Gitea 认证服务：

```bash
./kc serve --home /tmp/kc-demo \
  --auth gitea --auth-url https://git.acme.example \
  --auth-admin gitea:1

export KC_SERVE=http://127.0.0.1:7380
export KC_AUTH_TOKEN='<alice-personal-access-token>'
unset KC_AS
dsh --profile dsh-loom
```

`KC_AUTH_TOKEN` 是当前 DSH 用户的登录凭证，只随请求发送；不要把它设成服务访问远端知识仓使用的 `KC_GITEA_TOKEN`。外部入口必须置于 TLS 反向代理后。

本地开发时装进 dsh profile：

```bash
cd dsh-plugin
npm install --legacy-peer-deps
npm run build

dsh plugin --profile dsh-loom add link:$PWD
# 默认连 http://127.0.0.1:7380；KC_WORKSPACE 仅预填新建表单
# 覆盖：export KC_SERVE=http://127.0.0.1:7380 KC_WORKSPACE=notes

dsh --profile dsh-loom
```

profile 自己的 `cordis.patch.yml` 可以整行覆盖 `loom-control` / `loom-fs` 的 config（后写的层赢）。正式发行包使用 `dsh plugin --profile dsh-loom add dsh-loom`；这里的 `link:` 只用于源码开发。

## 只换 ctx.fs

`cordis.patch.yml` 会关掉 dsh-base 的 `fs-sandbox`（本机磁盘），再插入 `loom-fs`。`ctx.shell` 仍是宿主机的：Loom 没有「在某个仓里跑命令」的概念，换 shell 会让命令和虚拟树脱节。`processPath()` / `fileUrl()` 返回 `loom://...` 占位，不是可打开的磁盘路径。

## 布局

```
src/
  client.ts   LoomVfs — 无框架的 HTTP 客户端，打 kc serve 的 vfs-* 动词
  control.ts  `kc` Agent tool、固定 actor/request context、惰性本地服务
  resource.ts `resource` Agent tool、固定身份与 Descriptor pin、运行服务调用
  skill.ts    在 ctx.skills 注册随包的 knowledge-catalog Skill
  tree.ts     把扁平路径列表变成目录语义
  text.ts     严格 UTF-8 + 字面 search/replace
  errors.ts   kc ErrorCode → dsh-fs FsErrorCode
  fs.ts       LoomFileSystem extends FileSystem — 真正的 ctx.fs
  web.ts      DSH host 的只读 VFS HTTP 桥（认证信息不进浏览器）
  browser.tsx Workspace 导航内的 VFS 目录 + DSH Catalog 内容页
  index.ts    导出
test/
  tree.test.ts / text.test.ts / client.test.ts / control.test.ts / resource.test.ts / skill.test.ts / web.test.ts
  integration.test.ts  拉起真实 kc serve，驱动真实 LoomFileSystem
skills/knowledge-catalog/SKILL.md  Agent 的完整操作说明与安全边界
skills/integration-development/   Agent 开发 Collector 与资源访问包的严格运行契约
scripts/e2e-agent-roles.sh         空目录、六角色、真实模型验收
```

## 测试

```bash
npm test
# 集成测试需要已编译的 kc：
#   go build -o ../kc-test-bin ./cmd/kc
#   KC_BIN=../kc-test-bin npm test
# 没有二进制时 integration 套件会 skip，纯函数测试仍跑。
```

## 真实 dsh agent

`npm test` 只驱动 `LoomFileSystem` + `kc serve`，不经过 dsh 的 tool/policy。完整发布/消费：

```bash
# 本机已装 dsh（0.1.0-rc.7+）。默认从 $HOME/lore/.env 读 OPENAI_API_KEY / OPENAI_BASE_URL
./scripts/e2e-dsh.sh
```

这会用两个真实 headless agent：

1. **发布者**（主人，无 `--as`）把知识文件写进 Catalog：`analysis/notes.md` 和 `.dsh/skills/notes-ops/SKILL.md`（skill 也是知识文件）。
2. **发权**：`bob` 只有 `read-workspace` + `read`，没有 `commit`。
3. **消费者**（`KC_AS=bob`）跟 skill 读笔记；对同一棵树的 `vfs-write` 必须 `FORBIDDEN`。

覆盖密钥文件：`LORE_ENV=/path/to/.env ./scripts/e2e-dsh.sh`。

六角色治理闭环验收：

```bash
./scripts/e2e-agent-roles.sh
```

该脚本会创建全新的 `KC_HOME` 和六个独立空工作目录，由真实 DSH Agent 完成 Owner → Producer → Reviewer → Consumer → Auditor → Unauthorized Actor。它还会检查每个 session 的 trace，证明 Agent 实际调用了 bundled `knowledge-catalog` Skill 和 `kc` 工具；独立 oracle 再验证 Proposal 未提前移动 main、合并后的值与来源、未授权写入前后 HEAD 不变。证据目录会在结束时打印。

## 已知粗糙边

`@deepseek-ai/dsh-fs` 的若干运行时依赖只写在 peerDependencies 里，更严的 `npm install` 可能解不开。`npm install --legacy-peer-deps` 即可构建和测试。对上本机 `dsh` 0.1.0-rc.7 时，peer 也要对齐同一条线，否则 `FileSystem` / `FsError` 会变成两份 class。
