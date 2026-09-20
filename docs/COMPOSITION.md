# 多仓组合层

日期：2026-08-25
具体类型和命令的**已选定形状**看 `catalog/README.md`。公开名称是 Catalog / 知识集（`TERMINOLOGY.md`）；**Loom 不是公开别名。**

本文解释为什么 Knowledge Catalog 需要一个不理解知识正文的多仓组合层，以及为什么采用显式 mount、命令内 pin 和按路径唯一写回。

---

## Goal

解释为什么需要一个不理解知识正文的多仓组合层：显式 mount、命令内 pin、按路径唯一写回，把多个权威织成一棵工作树而不是拷成 monorepo。

## Non-Goals

- 不合成 monorepo、不把上游拷进 personal、不伪造单一版本历史（下文「为什么」）。
- 组合层不认识 `object_id`；挂普通 Git 不必先补 Schema（下文「组合层必须可裸用」）。VFS 是 pin 上的文件消费能力，不是 Catalog 成员条件；关掉 Plain 仓不取消对**知识仓**固定 commit 的文件投影。
- 知识集不是可写仓，也不发权（`PERMISSIONS.md`；`KS-02`）。发布把 selector 冻成 commit；之后跟分支要再发一版。

## 硬性约束 / Invariants

- `KS-01` `ResolvedKnowledgeSet` 冻结某 Dataset 版本的文件清单（从而导出 `{repository → commit}`），不复制、不覆盖。
- `V-01` 一次命令只解析一次 `latest` / 指定 `vN`，命令内不跟随 live 分支。
- `W-01` 写回必须路由到唯一成员 Repository。
- pin 锁数据坐标，不冻结未来权限（`KS-02`）。
- Catalog 身份、成员登记、知识集配方与生命周期由独立的 Catalog Snapshot 权威持久保存（树 + ref CAS，不是 Knowledge Repository）；替换 Server 或清空工作缓存不能改变它们。
- mount 路径显式声明；任意路径最多属于一条 mount；同一 Repository 的多条 mount 共享 selector/baseRev/commit，且成员 `subPath` 不得重叠。

## 选定方案 / 被否决方案

- 选定：[ADR-008](KNOWLEDGE_CATALOG_DESIGN.md#adr-008) / [ADR-009](KNOWLEDGE_CATALOG_DESIGN.md#adr-009) / [ADR-010](KNOWLEDGE_CATALOG_DESIGN.md#adr-010)；路径归属由 mount 配方决定。
- 选定：服务从既有 Catalog Snapshot 权威恢复组合态；接入既有知识 Snapshot 时先只读验证，再原子提交成员登记。物理 binding 属于服务管理的持久连接配置，不进入 Catalog 协议类型；接入方通过客户端管理获准的连接，不应逐仓依赖部署方修改配置文件。
- 选定：Catalog 权威与知识仓使用同一类 Snapshot 介质（部署可选 dolt / gitea / lakefs），但是单独的 repository；登记表禁止落在实例工作盘或知识仓树内。
- 否决：用本机目录发现代替持久 Catalog；把重新部署解释为重新创建组合空间；把 create
  隐式解释成已经 attach；为 Catalog 另开 SQL；把登记表写入成员知识仓。
- 选定：显式平台仓创建由应用管理面供给 Snapshot、保存连接并执行创建者授权策略；随后
  `attach` 才用 Catalog 成员准入合同登记。Catalog 核心不承担存储供给或保存连接秘密。
- 否决（本文边界）：union mount 改写用户工作区；根 mount 附着模式（本文后文对照表）。系统级拒绝见系统设计 [R-05](KNOWLEDGE_CATALOG_DESIGN.md#r-05)。

## 接口契约 / 状态机

`catalog` 协议：KnowledgeSet / Resolve 一次 / 按路径写回唯一成员。参考实现：`catalog/`、`catalog/worktree/`。配方便携文件是成员仓根 `.kc-dataset.yaml`。公开名称见 `TERMINOLOGY.md`。README 不是登记表字段；组合层仍不认识 `object_id`。


## 1. 为什么要织不要拷

真实工作不会只用一个 Repository：组织政策、团队文档、个人工作和外部代码常由不同权威维护。Agent 又希望看到一棵连续的工作树，并把修改写回正确仓。

直接合成 monorepo 会产生四个问题：

- 权限边界被抹平；
- 上游内容被复制，产生同步责任；
- 多仓历史被伪造成单一版本；
- 一次写入的唯一 target 不再明确。

因此需要“织”而不是“拷”：

```text
多个独立 Repository
      ↓ Knowledge Set recipe
命令内固定的多仓坐标
      ↓ checkout / virtual file projection
一棵连续工作树
      ↓ path routing
修改回到唯一成员仓
```

---

## 2. 推导

### 2.1 组合层必须可裸用

挂普通 Git 仓只需要 ⓪ Snapshot 与 ① Catalog。用户不应先补 `object_id`、Schema 或 Aspect 才能组合、检出和按路径工作。VFS / File Gateway 是已经固定的 pin 上怎么把树给人看，不因此要求把仓登记进 Catalog，也不把普通文件变成知识。Plain 仓只解释这道组合阶梯：可以 pin 一棵还没有 Schema 的树。它不是「能挂文件所以必须收 Plain」；知识仓的固定 commit 照样可以挂 VFS。

这形成采用阶梯：

```text
普通 Git               mount / pin / checkout
+ 知识 frontmatter     READ / provenance / Aspect
+ field access[]       SEARCH / AccessSpec / RetrievalPlan
```

能力缺失在真正使用那一层时报告，不在挂载时预先阻止。

### 2.2 配方和本次坐标分开

KnowledgeSet 保存成员、selector 和路径布局。**发布**把 selector 冻成 commit 与文件清单；之后上游前进不会改写这一版。再发一版（新 revision / `vN`）才跟上。`ResolveKnowledgeSet` 在命令开始时只解这一次已发布坐标（`latest` 或指定 `vN`），命令内不跟随 live 分支。

```text
KnowledgeSet --publish freeze--> Dataset vN file list
                --resolve once--> ResolvedKnowledgeSet
```

pin 默认不落盘；需要跨任务重放时可以显式导出。它锁数据坐标，不授予或冻结未来权限。

### 2.3 路径归属与对象身份分开

组合层不能认识 `object_id`：同一个知识对象可以在多个仓中拥有不同来源的 Aspect 或 Assertion，但一个文件路径必须有唯一写回落点。

```text
路径属于哪个仓       ① mount 配方决定
文件表示哪个对象     ② 内容解释决定
```

如果 ① 尝试按 `object_id` 决定归属，就必须回答“多来源的同一对象到底属于哪个仓”，这个问题本来没有唯一答案。

### 2.4 写回必须可逆

mount 只允许路径前缀重映射，不引入任意 filter 代数：

```text
member subPath  ↔  knowledge-set path prefix
```

纯前缀替换天然可逆，任何路径最多落到一个成员。复杂 filter 会把组合层变成第二种查询语言，并让反向写回需要猜测。

### 2.5 组合不制造跨仓事务

知识集本身不可写。跨 mount 修改拆成多次单仓提交；第二个仓失败不回滚第一个仓。系统必须如实暴露部分完成，而不是伪造原子性。

---

## 3. 配方、pin 与宿主视图

### 3.1 知识集配方

一条 mount 只声明四个概念：成员 Repository、selector、工作树路径、成员子路径。同一 Repository 可投影多个不重叠的子目录，但这些 mount 必须共享 selector/baseRev；因此一次 pin 对该仓仍只有一个 commit。发布时 selector 被冻成该 commit，pin 就是这一版的文件清单。便携配方跟着成员仓的 Git 历史；本机 overlay 只影响当前 principal，不进入共享配方或 Catalog 登记表。

具体 YAML 和字段约束由 `catalog/recipe.go`、`catalog/definition.go` 及测试描述，本文不复制。

### 3.2 Pin

凡决定本次 Snapshot 读结果的坐标都进入 PinID：成员 commit 与路径布局。配方 revision 本身不替代内容坐标。

State/Stream Binding 的 observation basis 由上层 Retrieval 请求持有，不进入知识集 PinID；否则 Catalog 就必须解释动态运行时。

Preview 在同一次已解析坐标上叠 Candidate overlay；结构校验确保成员 Repository 已接入且 commit 可用。

### 3.3 三种宿主视图

物理检出适合编辑器和直接操作文件的 Agent：每条 mount 保留自己的真实 Git 工作区，冲突、status 和历史仍由该成员 Git 处理。知识集根不是一个伪造的 Git 仓。

Linux 主机挂载适合“用户已有工作区 + 有限知识目录”的场景：`kcfs` Resolve 一次知识集，然后把每条非根 `Path` 分别作为只读 FUSE mount 挂到 `<用户工作区>/<Path>`。它使用 BSD 许可的 [go-fuse/v2](https://github.com/hanwen/go-fuse) 处理 FUSE 协议，内容仍来自成员在固定 commit 上的 `snapshot.TreeStore`。用户、IDE、shell、`rg` 和 Agent 因此看到同一棵真实宿主文件树；DSH 不再实现另一套 `read/list/glob/grep`。

VFS 仅适用于本机放得下的 Dataset，受本机存储与资源容量约束，不适合特别大的数据集。
它提供本地 Agent 可复用的只读文件入口，目前不支持通过挂载目录修改或写回。
下文的按需回读机制不是超出本机容量的产品承诺；大规模数据消费使用服务端统一检索与按需读取，
或先切分出可在本机容纳的 Dataset。此处是产品采用边界，不表示挂载前必然全量下载。

```text
/work/my-app/                         用户原有目录（Git 或非 Git）
├── src/                              用户原有内容
├── docs/team/                        repo A@commit 的 FUSE mount
└── knowledge/policy/                 repo B@commit 的 FUSE mount
```

这不是 union mount：每个成员只占用配方声明的精确目录，因此不会复制或重写用户工作区，也不需要它采用特定布局。精确 mountpoint 必须不存在或为空；父目录及其它内容不受限制。根 mount 会隐藏整个用户目录，附着模式明确拒绝。挂载原语是目录，不得把单文件伪装成独立 mountpoint。这是组合合同，不是「Linux 首版没做」。

人用观察 UI 只读取 MountController 已批准的宿主挂载目录；远程非 POSIX 客户端使用正式 Knowledge Set File Gateway。二者都不替代 Agent 的文件系统，也不向 Agent 暴露 `vfs-*` 工具。

宿主投影使用按目录、带 continuation 的可选 Tree capability，并由各 authority 如实声明支持；`kcfs` lazy 回读，不在启动时枚举整棵树。不支持该能力的成员明确返回 `CAPABILITY_UNSATISFIED`，不能退回 `ListFiles` 全量扫描。大规模知识发现属于③ Retrieval；UI 虚拟滚动不能掩盖后端扫描。

### 3.4 权限边界

对命令式读取，每次调用按当前身份求值 Repository 权限。对落盘工作树，ACL 边界在 checkout 时：无权成员不能先落盘再指望后续命令阻止 Agent 直接读文件。

共享配方可能主动暴露某个 Repository identity；这不同于中心化列表旁路。内容仍由成员仓授权保护。

### 3.5 外部写权威

挂入的仓可能由外部系统直接 push。此时：

- 外部治理继续在外部系统；KC 不宣称夺走业务 git。
- Connector / typed HTTP / Agent 仍走 Writer；墙外不直写 Git。
- 人可以在自己仓里改 frontmatter，不经过 `kc writer` 这条 CLI。
- 一次提交要成为**已发布知识**，必须过与 Writer 同一套校验（保护分支、CI 代发 `kc writer commit`，或等价门）。认 published HEAD 可以；把绕过校验的直推当成已发布知识，不行。
- 投影仍对 published HEAD 对账；tree 上带知识 frontmatter 的文件按文件解释并进入索引，即使该 commit 没有 Writer 回执。`git watch` / webhook 不是正确性来源。
- 若直推已经变成 HEAD：索引会跟上，知识合同不会自动成立。Reader 解释失败或检索文档是垃圾，不是 Plain 仓能力，也不是「没索引」。
- 来源信封可能缺失，不能拿 Git author 冒充；后续写入的 Ref CAS 仍不能降级。

「不绕过 Writer」约束本工具控制的知识写入，以及**进 published 的那次知识发布**。它不把任意 git push 翻译成 COMMIT。

---

## 4. 场景推导

### 4.1 上游前进

下次任务重新 resolve；每条 mount 独立同步。无本地修改的成员可以前进，有未提交修改的成员停住并要求处理。整棵树没有单一 Git HEAD，只有一次多仓 pin。

### 4.2 冲突

冲突只属于发生修改的成员仓，并使用该仓的 Git 语义。组合层不发明跨仓三方合并。

### 4.3 多知识集共用成员

服务管理的 Repository 连接与 Catalog 成员登记可以被多个知识集复用；不同知识集可以在独立检出中固定不同已发布 commit，互不覆盖。新增知识集不应重复创建权威或把连接状态写成配方内容。

### 4.4 Agent 工作树

给不同身份的 Agent 生成不同 checkout。文件落盘后无法再靠 `kc grant add` 阻止它直接读取，因此授权裁剪必须发生在落盘之前。

---

## 5. 业界调研与取舍

下表保留调研方向与本项目取舍；机制细节需要按对应项目版本的一手文档核对。
除上文 go-fuse 的实现入口外，这里尚未给出足以逐项验证产品行为的来源，不能把类比视为证明。
下面的决定由唯一权威、固定版本和可逆写回约束推导，不依赖其它产品必须采用相同合同。

| 参照方向 | 本项目关注的机制 | 本项目取舍 |
|---|---|---|
| Android repo | 多项目配方、独立检出与分仓提交 | 配方与本次精确输入分开，pin 按需保存 |
| go-fuse | 标准文件系统接入与挂载生命周期 | 使用 FUSE 库；loopback 不成为知识写面 |
| rclone mount | 文件工具消费远端内容，以及缓存、写回的责任 | 缓存须有显式边界；close 不等于知识 COMMIT |
| josh | 路径投影的逆映射条件 | 只选可证明唯一目标的前缀映射，不引入任意 filter |
| Egeria | 写入落点与权威归属 | 明确目标，不按成员响应顺序猜测 |
| Solid | 内容留在各自权威以及应用访问的授权边界 | 本系统按 Repository 治理，不另引入资源级 ACL |
| Nix flakes | 可编辑配方与精确输入的分离 | 命令内固定版本，跨任务复核时显式保存 pin |

### 5.1 多项目配方与命令内 pin

以多项目工作树为参照，本系统需要同时保留配方、各仓历史与唯一写入落点。
配方表达长期选择；发布把当时的 selector 冻成 commit 与文件清单。命令开始时解析的 pin 保证本次读取一致；需要跨任务复核时再显式保存。
这解释了为什么已发布 Dataset 不跟 live 分支，而一次消费也不能中途跟随上游前进。再发一版才会看到之后的仓提交。

### 5.2 为什么只选择前缀映射

显式、互不重叠的 mount 前缀可以证明路径属于哪个仓，并给出唯一逆映射。
任意 filter 或历史投影需要另外证明这些性质，以及授权在转换后仍然成立。
本文选择前缀映射，接受表达能力受限的代价，避免让写回依赖未声明的转换规则。

### 5.3 为什么不能替用户猜落点

如果系统按注册顺序尝试成员，直到某个成员接受写入，目标就会随着成员能力或可用性变化。
同一个请求因此可能落到不同权威，破坏来源和写边界。显式 mount 路由在执行前确定目标，
目标不可用时明确失败。

---

## 6. 代码是具体协议说明

- 知识集类型、校验、pin：`catalog/definition.go`、`catalog/resolve.go`
- mount 路由：`catalog/mount.go`；固定 pin 上的裸文件读：`catalog/virtual.go`
- 宿主 git 检出 / 同步：`catalog/worktree/`
- Linux 多目录只读挂载：`datasetfs/`、`cmd/kcfs/`
- 便携配方：`catalog/recipe.go`
- 本机 overlay：`catalog/overlay.go`
- CLI/HTTP 动词：`cli/command.go` 与对应测试
- 外部虚拟文件接缝与 DSH 宿主使用：`snapshot/README.md`、`dsh-plugin/README.md`

已完成项和历史实现步骤不再维护在本文；代码、测试和 Git 历史已经提供更准确的证据。
