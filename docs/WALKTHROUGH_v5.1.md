# Knowledge Catalog 产品全流程

日期：2026-09-10

本文用已落地的产品 CLI 从登录、进入 Catalog、创建或连接 Repository，走到发布、读取、多源
组合与治理。argv 应然设计见 [`CLI.md`](CLI.md)；公开命令闭集见 [`cli/surface.go`](../cli/surface.go)，操作语义见
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
- `KS-01`：pin 只冻结 `{Repository → commit}`，不复制内容。
- `KS-02`：attach、Workspace 成员关系和旧 pin 都不发权。
- `W-01`：Writer 一次只写一个 Repository，目标显式且唯一。
- `K-06`：Merge 通过目标 Ref CAS，不伪造跨仓事务。

## 选定方案 / 被否决方案

- 选定：Server 地址和当前 Catalog 是 Client 上下文；日常命令不重复填写。
- 选定：`create` 只建立 KC 可打开的 Repository；`attach` 才改变 Catalog 库存。
- 选定：知识与 Writer 使用 `--repo` 或知识命令使用 `--dataset`，不接受 `--catalog` /
  `--source` / `--pin`。Writer 与 `schema list` 不接受 `--dataset`。
- 选定：Schema、对象历史和审计分页只用可选 `continuation` 表示下一页。
- 否决：旧 argv 别名、登录自动发权、Catalog 范围 SEARCH、产品 CLI 的 `kc pin` / `--pin`。

## 接口契约 / 状态机

```text
login
  → catalog list / use
  → create（KC 能打开）
  → attach（Catalog 承认）
  → writer（Repository 发布）
  → knowledge --repo（单仓消费）
  → dataset define（多仓配方）
  → knowledge --dataset（多仓消费；回执带 commit）
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
也不是 Catalog 权威历史；历史用 `catalog audit`。

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

草稿目录就是期望正文。发布由 Writer 对照当前版本求差后执行，command-id 由用户提供。提交前可看会改什么：

```bash
kc diff --repo kr://git.example/acme/knowledge --dir ./drafts

kc writer commit --command-id import-001 \
  --repo kr://git.example/acme/knowledge \
  --dir ./drafts

kc writer put --command-id note-002 \
  --repo kr://git.example/acme/knowledge \
  --object runbook/payment \
  --value '{"text":"切流前检查冻结窗口"}'
```

单 Repository 验收不建立 Workspace：

```bash
kc schema list --repo kr://git.example/acme/knowledge
kc read --repo kr://git.example/acme/knowledge \
  --object runbook/payment
kc search --repo kr://git.example/acme/knowledge \
  --query 冻结窗口
```

`kc schema list` 只说明该仓已发布有哪些实体（`entity`、可选 `description`、`objectId`）。合同走 `read --object schema/…`。页顶有 `repository`、`commit` 和可选 `continuation`。对象历史 `log` 同样以 continuation 是否存在表示下一页。

精确复核可显式传 `--commit` / `--ref`；默认 ref 使用 `snapshot.DefaultRef`。下一条命令会
重新观察已发布 HEAD，同一命令内部不会漂移。

## 4. 多 Repository 任务

只有任务同时读取两个以上 Repository 才定义 Dataset：

```bash
kc dataset define payments --revision 1 \
  --source kr://acme/policy \
  --source kr://acme/runbooks

kc search --dataset payments --query 冻结窗口
kc read --dataset payments --object policy/P-103
```

回执带 `repository` / `objectId` / `commit`。精确历史重放抄 `--repo --commit`。宿主文件投影对同一 Dataset 挂载，挂载时内部冻结：

```bash
kcfs plan --dataset payments --root /work/project
kcfs mount --dataset payments --root /work/project
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

kc governance preview create --proposal PR-42 --dataset payments
kc governance preview validate --preview <preview-id>
kc governance validation record --preview <preview-id> \
  --suite external-suite --outcome PASSED
kc governance proposal merge --proposal PR-42 --preview <preview-id>
```

Preview create 在本次命令解析 Dataset 一次并冻进 Preview。Merge 只推进
Proposal 目标 Repository 的 Ref。下一条 `search` / `read --dataset` 会按已发布配方再解析。

## 6. 运维与部署边界

Projection、AccessSpec、Hook、Gate、访问审计和反馈在 `operations` 分组；它们不是普通消费
前置。部署者使用：

```bash
kc deployment init --config deployment.yaml
kc deployment status --config deployment.yaml
kc serve --config deployment.yaml
```

`serve` 只恢复声明的耐久状态。Catalog Snapshot 权威、知识 Snapshot authority、stateDir 和可丢 cacheDir
相互独立；替换实例不能重新发权或改变 Repository 身份。

完整验证运行 `make test`；外部 Gitea、Dolt、OpenSearch 与 Linux/FUSE 再运行
`make test-all`。协议旅程位于 `.data/scenes/`，只通过公开 CLI 断言可观察状态。
