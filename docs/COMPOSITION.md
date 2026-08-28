# Loom：多仓组合层

日期：2026-08-25
状态：设计结论已落地；具体类型和命令看 `catalog/`、`catalog/README.md` 与 CLI 测试。产品名 **Loom** 仍是提议。

本文解释为什么 Knowledge Catalog 需要一个不理解知识正文的多仓组合层，以及为什么采用显式 mount、命令内 pin 和按路径唯一写回。

---

## 1. 问题

真实工作不会只用一个 Repository：组织政策、团队文档、个人工作和外部代码常由不同权威维护。Agent 又希望看到一棵连续的工作树，并把修改写回正确仓。

直接合成 monorepo 会产生四个问题：

- 权限边界被抹平；
- 上游内容被复制，产生同步责任；
- 多仓历史被伪造成单一版本；
- 一次写入的唯一 target 不再明确。

因此需要“织”而不是“拷”：

```text
多个独立 Repository
      ↓ Workspace recipe
命令内固定的多仓坐标
      ↓ checkout / virtual file projection
一棵连续工作树
      ↓ path routing
修改回到唯一成员仓
```

---

## 2. 第一性原理

### 2.1 组合层必须可裸用

挂普通 Git 仓只需要 ⓪ Snapshot 与 ① Catalog。用户不应先补 `object_id`、Schema 或 Aspect 才能组合、检出和按路径工作。

这形成采用阶梯：

```text
普通 Git               mount / pin / checkout
+ 知识 frontmatter     READ / provenance / Aspect
+ field access[]       SEARCH / AccessSpec / RetrievalPlan
```

能力缺失在真正使用那一层时报告，不在挂载时预先阻止。

### 2.2 配方和本次坐标分开

WorkspaceDefinition 保存成员、selector 和路径布局；ResolveWorkspace 在命令开始时把 selector 各解析一次。配方可以跟分支，本次任务不能中途漂移。

```text
WorkspaceDefinition --resolve once--> ResolvedWorkspace
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
member subPath  ↔  workspace path prefix
```

纯前缀替换天然可逆，任何路径最多落到一个成员。复杂 filter 会把组合层变成第二种查询语言，并让反向写回需要猜测。

### 2.5 组合不制造跨仓事务

Workspace 本身不可写。跨 mount 修改拆成多次单仓提交；第二个仓失败不回滚第一个仓。系统必须如实暴露部分完成，而不是伪造原子性。

---

## 3. 设计决策

### 3.1 Workspace 配方

一条 mount 只声明四个概念：成员 Repository、selector、工作树路径、成员子路径。同一 Repository 可投影多个不重叠的子目录，但这些 mount 必须共享 selector/baseRev；因此一次 pin 对该仓仍只有一个 commit。便携配方跟着成员仓的 Git 历史；本机 overlay 只影响当前 principal，不进入共享配方或 Catalog 登记表。

具体 YAML 和字段约束由 `catalog/recipe.go`、`catalog/definition.go` 及测试描述，本文不复制。

### 3.2 Pin

凡决定本次 Snapshot 读结果的坐标都进入 PinID：成员 commit 与路径布局。配方 revision 本身不替代内容坐标。

State/Stream Binding 的 observation basis 由上层 Retrieval 请求持有，不进入 Workspace PinID；否则 Catalog 就必须解释动态运行时。

Preview 在同一次已解析坐标上叠 Candidate overlay；结构校验确保成员 Repository 已接入且 commit 可用。

### 3.3 三种宿主视图

物理检出适合编辑器和直接操作文件的 Agent：每条 mount 保留自己的真实 Git 工作区，冲突、status 和历史仍由该成员 Git 处理。Workspace 根不是一个伪造的 Git 仓。

Linux 主机挂载适合“用户已有工作区 + 有限知识目录”的场景：`kcfs` Resolve 一次 Workspace，然后把每条非根 `Path` 分别作为只读 FUSE mount 挂到 `<用户工作区>/<Path>`。它使用 BSD 许可的 [go-fuse/v2](https://github.com/hanwen/go-fuse) 处理 FUSE 协议，内容仍来自成员在固定 commit 上的 `snapshot.TreeStore`。用户、IDE、shell、`rg` 和 Agent 因此看到同一棵真实宿主文件树；DSH 不再实现另一套 `read/list/glob/grep`。

```text
/work/my-app/                         用户原有目录（Git 或非 Git）
├── src/                              用户原有内容
├── docs/team/                        repo A@commit 的 FUSE mount
└── knowledge/policy/                 repo B@commit 的 FUSE mount
```

这不是 union mount：每个成员只占用配方声明的精确目录，因此不会复制或重写用户工作区，也不需要它采用特定布局。精确 mountpoint 必须不存在或为空；父目录及其它内容不受限制。根 mount 会隐藏整个用户目录，附着模式明确拒绝。FUSE 的可移植挂载原语是目录，所以首版不把单文件伪装成独立 mountpoint。

人用观察 UI 只读取 MountController 已批准的宿主挂载目录；远程非 POSIX 客户端使用正式 Workspace File Gateway。二者都不替代 Agent 的文件系统，也不向 Agent 暴露 `vfs-*` 工具。

宿主投影使用按目录、带 continuation 的可选 Tree capability，并由各 authority 如实声明支持；`kcfs` lazy 回读，不在启动时枚举整棵树。不支持该能力的成员明确返回 `CAPABILITY_UNSATISFIED`，不能退回 `ListFiles` 全量扫描。大规模知识发现属于③ Retrieval；UI 虚拟滚动不能掩盖后端扫描。

### 3.4 权限边界

对命令式读取，每次调用按当前身份求值 Repository 权限。对落盘工作树，ACL 边界在 checkout 时：无权成员不能先落盘再指望后续命令阻止 Agent 直接读文件。

共享配方可能主动暴露某个 Repository identity；这不同于中心化列表旁路。内容仍由成员仓授权保护。

### 3.5 外部写权威

挂入的仓可能由外部系统直接 push。此时：

- 外部治理继续在外部系统；
- 本工具的 Hook/Gate 不能声称覆盖外部 push；
- 投影需要主动对齐；
- 来源信封可能缺失，不能拿 Git author 冒充；
- 后续写入的 Ref CAS 仍不能降级。

“不绕过 Writer”约束本工具控制的知识写入，不宣称夺走所有外部 Git 的写权威。

---

## 4. 不变量

1. mount 路径显式声明；没有隐式写回归属。
2. 任意路径最多属于一条 mount；挂载点不得重叠。
3. 同一 Repository 可有多条 mount，但只有一个 selector/baseRev/commit，且成员 `subPath` 不得重叠。
4. 路径决定落点，内容决定知识身份。
5. 一次写一个成员仓；不做跨仓事务。
6. selector 每条命令解析一次，中途不跟随 latest。
7. 配方不发权。
8. 组合层不解释 `object_id`、Aspect、Schema 或 provenance。
9. mount 是织，不是复制。
10. 不使用本工具的人仍能把成员仓当普通 Git 使用。

---

## 5. 场景推导

### 5.1 上游前进

下次任务重新 resolve；每条 mount 独立同步。无本地修改的成员可以前进，有未提交修改的成员停住并要求处理。整棵树没有单一 Git HEAD，只有一次多仓 pin。

### 5.2 冲突

冲突只属于发生修改的成员仓，并使用该仓的 Git 语义。组合层不发明跨仓三方合并。

### 5.3 多 Workspace 共用成员

Repository 只需接入本机 Store Directory 一次；不同 Workspace 可以在独立检出中固定不同 selector/commit，互不覆盖。

### 5.4 Agent 工作树

给不同身份的 Agent 生成不同 checkout。文件落盘后无法再靠 `kc admin grant add` 阻止它直接读取，因此授权裁剪必须发生在落盘之前。

---

## 6. 业界调研与取舍

| 项目 | 借鉴 | 不采用 |
|---|---|---|
| Android repo | manifest、多 project 检出、分仓提交、本机 overlay、revision lock | 不把 pin 默认提交成永久 lock |
| go-fuse | Linux FUSE 协议、高层 `fs` API、成熟 mount/unmount 生命周期 | 不自写 `/dev/fuse` wire protocol；不采用其 loopback 作为知识写面 |
| rclone mount | 远端数据通过标准文件系统给任意工具消费、显式 cache/write-back 模式 | 不引入面向对象存储的 VFS/cache 语义；不把 close 当知识 COMMIT |
| josh | 路径投影必须可逆；单 target 写回 | 任意 filter、把多独立权威合成一仓 |
| Egeria | home repository 说明写落点属于权威边界 | 依次尝试成员直到有人接受的猜测式路由 |
| Solid | 数据留在原权威，应用去访问 | 资源级 ACL 复杂度；本系统按 Repository 治理 |
| Nix flakes | 配方与精确输入分开 | 构建 lock 默认持久化；本系统 pin 默认只服务一次读 |

### 6.1 Android repo 是主要参照

它长期验证了“统一工作树 + 独立 Git 项目 + 分仓写回”可行。我们直接采用其 manifest/local override/base revision 思路，但保留命令内 pin，因为知识消费通常要求一次读一致，而不是默认制造长期 lockfile。

### 6.2 josh 说明为什么不能扩成 filter 语言

josh 能反向 push，是因为所有投影最终只有一个上游 monorepo。多权威场景没有这个前提；若引入任意 filter 与历史投影，就会同时破坏权限边界、唯一 target 和可逆写回。

### 6.3 Egeria 说明不能替用户猜落点

home repository 是合理的权威归属；“本地不支持就按注册顺序尝试远端”则会让相同请求在不同时刻落到不同仓。显式 mount 路由避免这种不确定性。

---

## 7. 代码是具体协议说明

- Workspace 类型、校验、pin：`catalog/definition.go`、`catalog/resolve.go`
- mount 路由与检出：`catalog/checkout.go`
- Linux 多目录只读挂载：`workspacefs/`、`cmd/kcfs/`
- 便携配方：`catalog/recipe.go`
- 本机 overlay：`catalog/overlay.go`
- CLI/HTTP 动词：`cli/command.go` 与对应测试
- 外部虚拟文件接缝与 DSH 宿主使用：`snapshot/README.md`、`dsh-plugin/README.md`

已完成项和历史实现步骤不再维护在本文；代码、测试和 Git 历史已经提供更准确的证据。
