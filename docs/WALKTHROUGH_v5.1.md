# Knowledge Catalog 产品全流程

日期：2026-09-10

本文用已落地的产品 CLI 从登录、进入 Catalog、创建或连接 Repository，走到发布、读取、多源
组合与治理。公开命令闭集见 [`cli/surface.go`](../cli/surface.go)，操作语义见
[`cli/SURFACE.md`](../cli/SURFACE.md)。

## Goal

给真人一条不暴露部署内部坐标的最短路径，并说明每一步改变哪份耐久状态：Client 会话、
Repository、Catalog、Workspace pin 或治理状态。

## Non-Goals

- 不把 HTTP 资源层级镜像成产品 argv。
- 不提供知识对象 LIST、Snapshot export 或本机 Home 旁路。
- 不让 create/attach/Workspace 隐式发 `knowledge.read`。
- 不把单 Repository 消费包装成 Workspace。
- 不把 Proposal 治理当成普通 Writer COMMIT。

## 硬性约束 / Invariants

- `V-01`：一次命令只解析一次 selector；后续步骤复用同一 pin。
- `WS-01`：pin 只冻结 `{Repository → commit}`，不复制内容。
- `WS-02`：attach、Workspace 成员关系和旧 pin 都不发权。
- `W-01`：Writer 一次只写一个 Repository，目标显式且唯一。
- `K-06`：Merge 通过目标 Ref CAS，不伪造跨仓事务。

## 选定方案 / 被否决方案

- 选定：Server 地址和当前 Catalog 是 Client 上下文；日常命令不重复填写。
- 选定：`create` 只建立 KC 可打开的 Repository；`attach` 才改变 Catalog 库存。
- 选定：知识与 Writer 使用 `--repo` 或 `--pin`，不接受 `--catalog` / `--workspace` /
  `--source`。
- 选定：Schema、对象历史和审计分页只用可选 `continuation` 表示下一页。
- 否决：旧 argv 别名、登录自动发权、Catalog 范围 SEARCH、Preview 再解析命名 Workspace。

## 接口契约 / 状态机

```text
login
  → catalog list / use
  → create（KC 能打开）
  → attach（Catalog 承认）
  → writer（Repository 发布）
  → knowledge --repo（单仓消费）
  → workspace pin（仅多仓）
  → knowledge --pin（固定多仓消费）
```

## 1. 登录、进入 Catalog 与查权

Server 地址来自 `--server`、`KC_SERVER_URL` 或保存的 Client 配置。命令只需在切换部署或首次
配置时覆盖地址：

```bash
kc login
kc whoami
kc admission show
kc catalog list
kc catalog use kr://acme/catalog   # 只有一间可见时可省
kc show
```

登录回执只说明认证结果和 principal，不重复 Server。`admission show` 返回本人当前 grants、
外部申请 URL（若配置）和当前 grant 管理者；KC 不承载申请/审批队列，也没有
`admission request`。

`show` 是当前 Catalog 的组合态：只有已 attach 的 Repository 和命名知识集。它不是对象目录，
也不是 Catalog Git 历史；历史用 `catalog audit`。

## 2. 创建、连接与 attach

平台供给：

```bash
kc create --name 团队知识
# 可选：--store <部署允许的池>
```

自有 Repository：

```bash
kc create --url https://git.example/acme/knowledge.git \
  --credential-file ~/.config/kc/git-token
```

两种 create 互斥。产品 argv 不接受 Catalog、Repository ID 或 command-id；返回值给出生成的
Repository 身份。create 完成后 `show` 仍看不到它：

```bash
kc attach --repo kr://git.example/acme/knowledge
kc show
```

attach 不要求 Repository 已经发布，不发任何知识读权。拿下组合关系：

```bash
kc detach --repo kr://git.example/acme/knowledge
```

detach 不删除 Snapshot 或对象。发权只有一条产品入口：

```bash
kc grant add --repo kr://git.example/acme/knowledge \
  --principal alice \
  --action knowledge.read,knowledge.search,knowledge.schema.read
kc grant list
kc grant remove --id <rule-id>
```

`grant add` 的 `--repo` 与 `--catalog` 必须二选一。

## 3. 发布并按 Repository 回读

草稿目录先在 Client 收成 ChangeSet：

```bash
kc pack --repo kr://git.example/acme/knowledge \
  --dir ./drafts --out changeset.json
```

`pack` 不连接 Server、不发布。发布由 Writer 执行，command-id 由用户提供：

```bash
kc writer commit --command-id import-001 --changeset changeset.json

kc writer put --command-id note-002 \
  --repo kr://git.example/acme/knowledge \
  --object runbook/payment \
  --value '{"text":"切流前检查冻结窗口"}'
```

单 Repository 验收不建立 Workspace：

```bash
kc knowledge schema list --repo kr://git.example/acme/knowledge
kc knowledge read --repo kr://git.example/acme/knowledge \
  --object runbook/payment
kc knowledge search --repo kr://git.example/acme/knowledge \
  --query 冻结窗口
```

Schema 页返回顶层 `repository`、`commit`、`schemas` 和可选 `continuation`。每条 Schema 不再
重复 Repository/commit，也没有 `coverage`、`exhausted` 或 `total`。对象历史
`knowledge log` 同样以 continuation 是否存在表示下一页。

精确复核可显式传 `--commit` / `--ref`；默认 ref 使用 `snapshot.DefaultRef`。下一条命令会
重新观察已发布 HEAD，同一命令内部不会漂移。

## 4. 多 Repository 任务

只有任务同时读取两个以上 Repository 才先 pin：

```bash
kc workspace pin \
  --source kr://acme/policy \
  --source kr://acme/runbooks \
  --out pin.json

kc knowledge search --pin pin.json --query 冻结窗口
kc knowledge read --pin pin.json --object policy/P-103
```

共享命名配方是进阶组合操作：

```bash
kc workspace define payments --revision 1 \
  --source kr://acme/policy \
  --source kr://acme/runbooks
kc workspace pin --workspace payments --out pin.json
```

保存的 pin 携带 Catalog 与 Workspace 坐标。宿主文件投影只接受这份固定输入：

```bash
kcfs plan --pin pin.json --root /work/project
kcfs mount --pin pin.json --root /work/project
```

挂载目录只读；用户工作目录其它部分仍由普通文件工具处理。

## 5. Proposal 治理

普通同步继续使用 Writer。需要候选、验证和 Merge 时：

```bash
kc governance proposal create \
  --proposal-id PR-42 \
  --repo kr://acme/runbooks \
  --candidate refs/heads/candidates/PR-42 \
  --object runbook/payment \
  --value '{"text":"候选正文"}'

kc workspace pin --workspace payments --out preview-pin.json
kc governance preview create --proposal PR-42 --pin preview-pin.json
kc governance preview validate --preview <preview-id>
kc governance validation record --preview <preview-id> \
  --suite external-suite --outcome PASSED
kc governance proposal merge --proposal PR-42 --preview <preview-id>
```

Preview 必须叠在调用方给出的固定 pin 上，不能用 `--workspace` 重新解析 latest。Merge 只推进
Proposal 目标 Repository 的 Ref；其它成员保持 pin 中的 commit。下一条知识命令重新 pin 后才
看到新发布版本。

## 6. 运维与部署边界

Projection、AccessSpec、Hook、Gate、访问审计和反馈在 `operations` 分组；它们不是普通消费
前置。部署者使用：

```bash
kc deployment init --config deployment.yaml
kc deployment status --config deployment.yaml
kc serve --config deployment.yaml
```

`serve` 只恢复声明的耐久状态。Catalog Git、Snapshot authority、stateDir 和可丢 cacheDir
相互独立；替换实例不能重新发权或改变 Repository 身份。

完整验证运行 `make test`；外部 Gitea、Dolt、OpenSearch 与 Linux/FUSE 再运行
`make test-all`。协议旅程位于 `.data/scenes/`，只通过公开 CLI 断言可观察状态。
