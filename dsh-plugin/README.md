# loom-dsh

DeepSeek Harness (`dsh`) 的 Agent-first Knowledge Catalog 插件。它同时提供：

- 随包安装、在空 Workspace 之前即可发现的 `knowledge-catalog` Skill；
- Agent 可直接调用的 `kc` 工具，复用 Go CLI 的同一张 HTTP 动词表；
- 把已解析 Workspace 暴露成文件树的 `ctx.fs` 与 `glob` / `grep` 工具。

用户表达目标后，Agent 可以自行初始化 Catalog、配置 Repo/Workspace/权限与门禁、写入或发起 Proposal、验证合并、读取检索、审计与追溯。UI 只是可选观察/审批面，不是核心路径。

agent 的 `read` / `write` / `edit` / `list` 走 Loom 的虚拟树（`kc serve` 的 `vfs-read` / `vfs-list` / `vfs-write`），按路径路由到各自的仓，**磁盘上不会检出一棵 checkout**。这和 `kc checkout`（真 git worktree）是两条路。

接缝对齐 [`@deepseek-ai/dsh-fs`](https://deepseek-harness.github.io/deepseek-harness/en/reference/subsystems/filesystem) 的 `FileSystem`，以及 [`docs/COMPOSITION.md`](../docs/COMPOSITION.md) §2.3.1。

## 从空目录开始

插件安装到 DSH profile 后，业务任务不要求用户预建 Catalog，也不要求另开终端启动服务：

```bash
mkdir empty-workspace && cd empty-workspace
export KC_HOME="$PWD/.kc-home"
export KC_WORKSPACE=agent

# 空 KC_AS 是本地 Workspace Owner；有角色时必须显式固定，例如 producer。
export KC_AS=
dsh --profile loom "建立知识目录并发布一条受治理的知识"
```

首次调用 `kc` 工具时，插件会在 loopback 上惰性启动 `kc serve`。`KC_BIN` 可指定现成二进制；否则随包脚本依次使用 PATH 中的 `kc`、本地 Knowledge Catalog 源码，最后从上游源码构建缓存。远程 `KC_SERVE` 不会被插件自动启动。

Agent 会先通过 DSH 的 Skill registry 加载 `knowledge-catalog`，其中包含空目录引导、Proposal/Preview/Validation/Merge、消费、审计、失败恢复和角色边界。模型不需要从 README 猜工作流。

## 身份与角色边界

当前没有身份认证。`KC_AS` / `X-Kc-As` 只是**不可信的 principal 声明**，用于验证授权语义和记录审计归属，不证明调用者真实身份。每个 Agent composition 固定一个 `KC_AS`；`kc` 工具会丢弃模型传入的 `as`、`home`、`listen`，因此被拒绝的角色不能静默去掉身份回退为 owner。每次调用同时携带 `X-Kc-Request-Id`。

建议为 Catalog Owner、Producer、Reviewer/Gatekeeper、Consumer、Auditor 和 Unauthorized Actor 分别启动独立 session/composition。Owner 是唯一使用空 principal 的本地引导角色；其它角色必须显式设置 `KC_AS`。

## Agent 看见什么

例如 alice 的 notes workspace：

```text
analysis/churn.md                 ← kr://acme/personals/alice
refs/semantic/metrics/dau.md      ← kr://acme/public/semantic
```

agent 只看到这一棵树，不传 `--repo`。落点由 mount 路径决定。无权的仓经 `X-Kc-As` / `kc allow` 裁剪。

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

本地开发时装进 dsh profile：

```bash
cd dsh-plugin
npm install --legacy-peer-deps
npm run build

dsh plugin --profile loom add link:$PWD
# 默认连 http://127.0.0.1:7380 、workspace notes
# 覆盖：export KC_SERVE=http://127.0.0.1:7380 KC_WORKSPACE=notes

dsh --profile loom
```

profile 自己的 `cordis.patch.yml` 可以整行覆盖 `loom-control` / `loom-fs` 的 config（后写的层赢）。正式发行包使用 `dsh plugin --profile loom add loom-dsh`；这里的 `link:` 只用于源码开发。

## 只换 ctx.fs

`cordis.patch.yml` 会关掉 dsh-base 的 `fs-sandbox`（本机磁盘），再插入 `loom-fs`。`ctx.shell` 仍是宿主机的：Loom 没有「在某个仓里跑命令」的概念，换 shell 会让命令和虚拟树脱节。`processPath()` / `fileUrl()` 返回 `loom://...` 占位，不是可打开的磁盘路径。

## 布局

```
src/
  client.ts   LoomVfs — 无框架的 HTTP 客户端，打 kc serve 的 vfs-* 动词
  control.ts  `kc` Agent tool、固定 actor/request context、惰性本地服务
  skill.ts    在 ctx.skills 注册随包的 knowledge-catalog Skill
  tree.ts     把扁平路径列表变成目录语义
  text.ts     严格 UTF-8 + 字面 search/replace
  errors.ts   kc ErrorCode → dsh-fs FsErrorCode
  fs.ts       LoomFileSystem extends FileSystem — 真正的 ctx.fs
  index.ts    导出
test/
  tree.test.ts / text.test.ts / client.test.ts / control.test.ts / skill.test.ts
  integration.test.ts  拉起真实 kc serve，驱动真实 LoomFileSystem
skills/knowledge-catalog/SKILL.md  Agent 的完整操作说明与安全边界
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
