# Loom：多仓组合层

日期：2026-08-21
状态：目标形态已定；§1.4 主链路已接通（挂仓、路径布局、检出、按路径写回、按 `--as` 裁剪）。产品名 **Loom** 是提议，语义是结论。
对照：`LAYERS.md`（⓪–③ 分层）、`PERMISSIONS.md`（发权）。本文取代原 `AUTHORING.md` 与 `CATALOG.md`。

---

## 0. 这是什么

**把若干独立的 git 仓，按声明的路径织成一棵可以直接开发的树；改动按路径路由回各自的仓。**

只用三个词：**仓 id、commit / ref、路径**。不读文件内容，不知道 `object_id`、Aspect、frontmatter、schema。

这是**底座**。知识层（②：对象身份、Aspect、来源信封）和检索层（③：声明式索引、结构化 SEARCH）都叠在它上面，但它自己必须能裸用——不叠任何东西也是个完整可用的工具。

Stream（APPEND / cursor）不属于这一层：它是 ⓪ 的流，这里只冻结坐标、不解释。

---

## 1. 目标形态

### 1.1 产品形态：装在自己机器上的工具

**装上它，手里有几个知识仓的 git link，挑一些挂进来，就得到一个可以开发的工作区。**

不需要先有公司级中心服务。`.kc/` 里的一切都长在本机；`kc init` 建的那间 Catalog 是**你的**工作台配置，不是全公司的注册中心。公司级形态只是很多人各自挂了同一批仓，不是另一套架构。

### 1.2 别人可以不用

硬约束：**知识仓必须是普通 git 仓，没装这个工具的人 clone 下来照样能读能写。** 不得引入任何只有本工具能解释的必需格式。任何「必须经过我们才能用」的设计都会杀死采用。

### 1.3 分层就是采用阶梯

| 对方仓的状态 | 能拿到什么 | 靠哪一层 |
|---|---|---|
| 任何 git 仓 | 挂进来、织进你的树、grep、编辑、按路径写回各自的仓 | **本层** |
| 加了 frontmatter `object_id` | 跨仓认同一个对象、`READ` 拼装、来源信封、`GET_PROVENANCE` | ② |
| 加了 `schema/*` AccessHints | 结构化 `SEARCH`、声明式索引、IndexPlan | ③ |

谁多走一步谁多拿一层，不需要任何人同时切换。这让「底座可裸用」从架构洁癖变成产品必需。

### 1.4 一个完整的例子

alice 写工作笔记，要引用组织政策。政策仓 `kr://example/org/policies` 在 Gitea 上，她有读权没写权；她自己有个笔记仓 `kr://example/personals/alice`。

**挂仓**——能不能挂取决于她的 git 凭证，这一层不管：

```bash
kc mount kr://example/personals/alice --link https://git.example.com/alice/notes.git
kc mount kr://example/org/policies --link https://git.example.com/example/policies.git
```

**定义 workspace**——所有 mount 显式声明路径，配方提交进 alice 的仓、跟着 git 走：

```yaml
# .kc-workspace.yaml
name: notes
mounts:
  - repository: kr://example/personals/alice
    selector: refs/heads/main
    path: ""                      # 挂在根：新增文件的兜底
  - repository: kr://example/org/policies
    selector: refs/heads/stable
    path: refs/policies
```

**检出**：

```bash
kc checkout --workspace notes --to ~/work/notes
```

```text
~/work/notes/
├── .kc-workspace.yaml
├── .kc-pin.json                    这次解开的坐标
├── daily/2026-08-21.md             ← 我的仓，可写
├── notes/review.md                 ← 我的仓，可写
└── refs/policies/                  ← 政策仓，只读（我没写权）
    ├── policies/retention.md
    └── policies/incident.md
```

**开发**就是正常编辑：改 `notes/review.md`、新建 `notes/follow-up.md`，一边写一边 `rg` 或直接打开 `refs/policies/policies/incident.md`。agent 也在这棵树上干活。

**提交**——落点由路径决定，没有一步需要告诉工具「我要写哪个仓」：

```bash
kc commit --workspace notes -m "add follow-up notes"
```

```text
kr://example/personals/alice   refs/heads/main   a1b2c3 → d4e5f6   2 files
```

**没写权时**如实挡住：

```text
error: 2 files under refs/policies/ belong to kr://example/org/policies (no write grant)
       use `kc propose --workspace notes --path refs/policies` or revert those files
```

**跨 mount 编辑**按仓拆，两个 receipt 各自 CAS，第二个失败不回滚第一个也不谎报成功：

```text
kr://example/personals/alice   main    a1b2c3 → d4e5f6   2 files
kr://example/org/policies      stable  9a8b7c → 1f2e3d   1 file
```

**别人的内容不进你的仓**：bob 直接 `git clone alice/notes.git`，拿到 `daily/`、`notes/`、`.kc-workspace.yaml`，**没有 `refs/policies/`**。那棵树是织出来的，不是拷贝。

---

## 2. 语义

### 2.1 名词

| 词 | 是什么 | 不是什么 |
|---|---|---|
| **Catalog** | 本机的承认表 + 一组 Workspace 配方，落盘是它自己的 git | 文件仓库；知识协议；中心服务 |
| **Repository** | 一个独立 git 仓的 id。独立 ACL、独立生命周期（K-02） | Workspace 的一部分；可按表拆权限的东西 |
| **Workspace** | 一棵树的配方：哪些仓、各挂在哪个路径、跟哪根 selector | 又一个仓；知识副本；权限发放 |
| **Mount** | 一条：`仓 + selector + 挂载路径 + 可选子路径` | 符号链接；filter 代数 |
| **Pin** | 一次解析的结果：`{仓 → commit}` + 路径布局 + 内容寻址的 `PinID` | 常驻 serving pointer；授权凭证 |

### 2.2 能力

**登记与挂载**：挂载仓（新建 / 指向已有本地路径 / 远程 link）、承认仓（可出现在配方里，**不发权**）、归档（没有 DELETE）、当前态（按 `--as` 裁剪）、登记表自己的历史。

**Workspace 定义**：

| 字段 | 语义 |
|---|---|
| `repository` | 挂哪个仓 |
| `selector` | 跟这个仓的哪根已发布 ref |
| `path` | 挂到树的哪个路径。`""` = 根（新增文件的兜底，可选） |
| `subPath` | 只挂仓里的哪个子目录。空 = 整仓（P1） |

配方文件 `.kc-workspace.yaml` 放仓根、跟着 git 走；无锚点的联邦 workspace 放本机或任一有写权的仓。本地叠加层（对标 `repo` 的 `local_manifests`）落在 `.kc/overlays/<principal>/<workspace>.yaml`，只在本机、只对这个 `--as` 生效（空 `--as` 是 `owner`）；不进成员仓 git，也不进 Catalog 登记表。

**解析与 Pin**：命令开始时把每个 selector 解成 commit，**命令内冻结、不落盘**；Preview 的 `{仓 → candidate}` overlay 叠在这次解析上；结构检查（成员已挂载、commit 存在）；配方层 CAS（`baseRev`：声明「我基于的成员 commit 应是 X」，被推走则解析失败，对标 `repo` 的 `base-rev`）；`PinID` 内容寻址、可按需导出重放。

**检出**：只读树（`rg` / 喂 agent）与可写树（编辑器和 agent 直接改）。

**写回路由**：路径 → 仓由 mount 前缀反查；仓内路径 = 去掉 `path` 前缀、加回 `subPath` 前缀；跨 mount 按仓拆成 N 次 COMMIT；无写权的 mount 拒绝或转 `propose`。

**权限**：`kc allow` 的仓级 ACL，作用对象是**你自己的 agent**。

### 2.3 三条关键判断

**这一层不能认 `object_id`**——不是暂时不引入，是引入了就打架：

```text
refs/policies/policies/incident.md frontmatter: object_id: policy/incident
notes/incident-notes.md            frontmatter: object_id: policy/incident   ← alice 对同一对象的补充
```

本层视角是两个文件、两个仓，归属清楚；② 的视角是一个对象、两个来源，`READ` 保留来源不覆盖。若本层感知 `object_id`，就得回答「`policy/incident` 属于哪个仓」，而它本来就同时属于两个仓，无解。**路径归属必须唯一，对象身份天然可以多来源**，只有分开才都成立。

**只允许前缀重映射，不做 filter 代数。** 一条 mount 就是「把仓的 `subPath` 子树整体搬到 `path` 下」，纯前缀替换，反向映射永远确定。josh 的复杂度几乎全部来自反转任意 filter 组合，它甚至需要 `--check-roundtrip` 验证可逆；限制成前缀映射，可逆性是免费的。这条要写死，否则会长成第二个 filter 语言。

**可写检出是各 mount 各自的真 git 工作区拼起来**，不是我们自己落一棵树。冲突落在各仓里就是普通 git 冲突，本层不发明冲突语义；自己造树等于要重造三方合并、status、stash、log。代价是 workspace 根目录本身不是 git 仓，根上 `git status` 不工作（用 `kc status`）——`repo` 就是这个形状。

实现上具体是 `git worktree add --detach`：每条 mount 是一个 linked worktree，跟它的 canonical 本地 clone（`.kc/repos/<id>`）共用 object store，零拷贝。选 `--detach` 不是图省事——`local.FileGitRepository.ApplyCommit` 本身会在 canonical 目录自己的工作区上 `checkout` 目标分支落地一次 commit；如果检出的 worktree 挂在分支上，Writer 写入时的分支切换会和它撞（git 不允许同一分支同时在两个工作区签出），detached 天然绕开这条限制。**这也带来一条限制，要显式承认**：linked worktree 靠 `.git` 文件指回 canonical 目录的 `.git/worktrees/<name>`，离不开那台机器、那个路径——检出的树不是可以 `rsync` 到别处还照样工作的独立产物，这跟 `repo`（每个 project 是独立 clone，理论上能搬）不同，是「本地工具」定位（§1.1）的自然代价，不是缺陷，但没写清楚就是文档缺口。

### 2.3.1 虚拟检出：给外部 agent harness 用，不落盘

可写检出（§2.3）解决的是「人或 agent 在磁盘上直接编辑」。还有一类消费者从不需要磁盘上的树，只需要「按路径读一个字节序列、按路径写一个字节序列」——典型是接入外部 agent harness 自己的文件系统抽象层（比如 DeepSeek Harness 的 `ctx.fs`，一个可替换的 provider seam）。给这类消费者也去 `git worktree add` 一整棵树是浪费：它们的每次文件操作本来就是一次独立调用，天然适合直接映射成一次 HTTP 往返。

这条路径不新增协议层，是把 Loom 已有的 `RouteMount`（路径 → 仓）接到一个新的、与 Knowledge（②）平级的**可选** ⓪ 能力上：

```text
repository.RawFileStore   可选能力，与 Knowledge 平级：按字面路径读写字节，不认 object_id、不解 frontmatter
  ReadFile(path, commit) []byte
  ListFiles(commit) []string
  ApplyRawCommit(RawFileChangeSet) commit      写走 Writer.RawWrite（新 Surface：RAW_WRITE），CAS/幂等/journal 与 COMMIT 同样机制
```

`local.FileGitRepository` 与 `gitea.Repository` 都实现了它（`internal/gitdir.ObjectType` 先判 blob/tree 再读，否则 `git show <rev>:<目录>` 会把目录列表当文件内容返回——这是本节唯一一处需要注意的坑）。`kc serve` 上对应三个动词：`vfs-read` / `vfs-list` / `vfs-write`，都是 `--workspace` + `--path`，落点由 `RouteMount` 现场决定，不传 `--repo`。`vfs-list` 同时返回文件条目和 mount 边界；每条 mount 带虚拟目录、Repository、selector、subPath 与本次 resolve 的 commit，空 mount 也可被观察面展示（仍按当前身份裁剪）。`vfs-write` 的 `--base` 精确对应 CAS 的 `expectedTargetCommit`：拿上次 `vfs-read`/`vfs-list` 返回的 `commit` 去写，写慢了得到 `NON_FAST_FORWARD`，不去重试就没有并发保护。

参考实现：`dsh-plugin/`（仓库根旁，TypeScript，不是协议代码）——一个 DeepSeek Harness `ctx.fs` provider，直接把 dsh 的 `read`/`write`/`edit`/`list` 工具调用翻成上面三个动词。它是这条虚拟路径「确实可以从外部真正接上」的验收，不是 Loom 协议本身的一部分，详见 `dsh-plugin/README.md`。

### 2.4 不变量

1. **所有 mount 显式声明路径**，包括根。没有隐式归属。
2. **路径唯一归属**：任何路径最多属于一条 mount，挂载点不得重叠或嵌套。
3. **一仓一挂**：同一个仓在一个 Workspace 里只出现一次。
4. **落点由路径决定，身份由内容决定**：文件属于哪个仓是本层的事实；文件是哪个对象是 ② 的解释。
5. **一次写一个仓**：跨 mount 的编辑拆成多次 COMMIT，不做跨仓事务（K-22）。
6. **命令内冻结**：selector 一次命令解一次，中途不跟随 `latest`。
7. **配方不发权**。
8. **不解释内容**。
9. **mount 是织不是拷**：别人的内容不进你的仓。
10. **不装本工具的人不受影响**。

### 2.5 不属于这一层

`object_id` / Aspect / frontmatter / `schema/*` / 来源信封 → ②；`READ` 拼装 / `SEARCH` / IndexPlan → ②③；APPEND / cursor → ⓪ 的流；表级 GRANT 与文件 ACL → 不做；filter 代数 / 历史投影 / 双向历史合并 → 不做；冲突合并算法 → 交给各 mount 自己的 git。

---

## 3. 场景

### 3.1 上游前进

再跑 checkout 就是重新 resolve，但**每条 mount 独立同步**：只读且无本地改动的直接换到新 commit；可写且有未提交改动的不动、提示先提交。整棵树没有统一的「版本」，只有各 mount 各自的位置。跟 `repo sync` 每个 project 独立 `git pull` 是一回事。

### 3.2 冲突

只发生在有写权且有本地改动的 mount 上，落在那个仓里，就是普通 git 冲突。本层不介入。

### 3.3 多个 workspace 共用同一个仓

**挂载在本机只有一次，检出是每个 workspace 一棵树。** selector 不同就可以停在不同 commit，互不干扰。git object 可共享存储（alternates），工作区独立。

### 3.4 agent 干活：边界在 checkout 时

树一旦落到磁盘，**文件系统上没有 ACL**，agent 直接读文件就绕过了 `kc allow`。所以两种消费方式的边界在不同位置：

| 方式 | 边界在哪 |
|---|---|
| 文件树（agent 直接读写） | **checkout 时**：用 `--as agent` 检出，无权的 mount 根本不落盘 |
| `kc read --workspace` 等命令 | 每次调用时求值 |

给 agent 的树和给自己的树是两次不同的 checkout、两个目录。这条不钉死，`allow` 在树上形同虚设。

### 3.5 团队共享配方

bob clone alice 的仓拿到配方，明文写着 `kr://example/org/policies`，他因此知道这个仓存在——哪怕没有读权。这不违反「无权与不存在不可区分」：那条约束的是**中心化列表**，不约束**用户主动分享的配方**。要保护的是内容，由那个仓自己的 ACL 保证。bob checkout 时那条 mount 落不下来应当**如实报告**，而不是假装不存在——他手里的配方已经写着它了。

### 3.6 挂别人的仓：写权威在外部

挂进来的仓，写权威可能在外部（用户自己的仓、远程 monorepo 投影）。此时外部直接 push 是**预期**，不是事故。要接受的降级：

| 面 | 后果 |
|---|---|
| gate | 只拦本工具发起的 `merge`，外部 push 绕不过；治理在外部系统 |
| hook | 外部 push 不触发 `pre`/`post` |
| 索引（③） | 增量索引挂在 `AfterSnapshot` 上，外部 push 不触发，靠 `index-sync` 对齐 |
| 来源信封 | 外部 commit 没有 provenance，不要用 git author 冒充 |
| CAS | **不降级**：Ref 被推走时再写照样 `NON_FAST_FORWARD` |

「不要绕过 Writer 直写 git」这条禁令的射程是**本工具自己建的仓**，不是宣称对所有挂进来的 git 有所有权。

---

## 4. 业界调研与取舍

| 项目 | 配方存哪 | pin 形态 | 写落点怎么定 | 权限落点 |
|---|---|---|---|---|
| **josh** | `workspace.josh`（被投影的历史里，自举） | 无（filter 是确定性函数） | 反转 filter，落点必然是那**一个** monorepo | proxy 进程；投影时过滤 |
| **Android repo** | 独立 git 仓里的 `default.xml` | `repo manifest -r`，**落盘** | 你改的文件在哪个 project 目录 | 各 remote 自己的 ACL |
| **Egeria** | cohort 注册（自配置 p2p） | 无版本坐标，live 查询 | **home repository**；create 依次试到有人接受 | 各成员自治 |
| **Solid** | SAI 的 Data/Application Registration | 无 | 资源所在的 pod | 资源级 WAC `.acl` |
| **Nix flakes** | `flake.nix` inputs | `flake.lock`，**落盘并提交** | 不适用（只读输入） | 无 |
| **Loom** | `.kc-workspace.yaml`（仓根，跟 git 走） | 命令内冻结，可按需导出 | mount 路径前缀反查 | 仓级 `kc allow`（约束 agent） |

**Android repo 几乎同构，是主要参照。** manifest 存在一个独立 git 仓里、`revision` 默认跟分支、`repo manifest -r` 出 revision-locked 快照——形状和我们一一对应。它三十年验证了最重要的一条：**统一检出 + 分仓写回**走得通，`repo upload` 一次一个仓，没有跨仓提交。两个机制直接借走：`local_manifests`（本地叠加自己的仓，不改公司配方）和 `base-rev`（配方层 CAS）。

**josh 不适用，但它印证了 K-01。** 官方文档明说 workspace 只在单一 monorepo 内工作，不跨独立上游仓。它能让你 clone 下来改完 push 回去，正是因为**落点必然唯一**——它没解决多 target 问题，是靠前提回避了。要把它用作我们的组合层有三处硬冲突：跨独立仓做不到（合成 monorepo 就等于放弃按治理边界拆仓，权限模型塌掉）；filter 代数建立在「路径即身份」上；workspace 产出有自己 HEAD 的真仓，且 mapped module 的历史会 merge 进来——既造出第二套坐标，又等于把别人的知识拷进自己的仓。

它有一个零成本的用法：josh-proxy 投影出的子仓对外就是普通 git remote，可以直接当我们的一条 mount 来源。「公司有个大 monorepo，我只想把 `docs/knowledge/` 当知识仓挂进来」这件事因此不需要写任何集成代码。

**Egeria 给了一正一反。** 正面是 **home repository**：每个实例有个「家」仓，维护请求路由到 home——这说明落点不是配方属性，而是**对象已经在哪儿**的事实。反面是它的 create 路由：本地不支持就依次试每个远程成员直到有人接受，落点取决于注册顺序和类型支持，同一请求在不同时刻可能落到不同仓。这正是我们禁止的「让协议替用户猜落点」。

**Solid 反向印证仓级 ACL。** 它的前提和个人知识场景完全一致（数据不进平台，应用来就数据），但选了资源级 ACL，走了快二十年——最新 SAI 规范里 `acl:Create`、`acl:Update`、`acl:Delete` 全标着 not currently supported，让你用更粗的 `acl:Write` 替代，规范自己注明 may exceed intended scope。业界最认真做资源级 ACL 的项目至今如此，是我们选仓级边界的强旁证。

**Nix flakes 的差异是有意的。** flake 和 repo 的 lock 都**默认落盘**，我们的 pin 是**按需导出**。理由：它们锁的是构建可复现性（跨时间的承诺，必须落盘），我们锁的是一次读的一致性（跨命令则跟已发布 selector）。pin 常驻落盘等于在 workspace 之外又造一个需要人工推进的指针。

**我们的位置**：这五个里没有任何一个同时具备——多仓组合配方（repo 有）、用户定义的路径布局且可写（josh 有但单仓）、跨仓联邦读且保留来源（Egeria 有但无版本坐标）、可收回的仓级授权（Solid 尝试了资源级，没做完）。这个组合没人做过。

---

## 5. 当前实现与目标的差距

| # | 现状 | 目标 | 动作 |
|---|---|---|---|
| ~~1~~ | ~~`Store.Add` 只收 SnapshotStore+Knowledge 兼备的仓——普通 git 仓类型上挂不进来~~ | 底座可裸用 | **已做**：`Store` 收 `SnapshotStore`；`Store.Knowledge` / `catalog.RequireKnowledge` 是通往 ② 的接缝；无 `schema_ref` 的 COMMIT 不再要求 ②；索引订阅遇裸仓跳过。见 `catalog/plain_member_test.go` |
| ~~2~~ | ~~`WorkspaceSource` 只有 `Repository` + `Selector`~~ | 全显式路径布局 | **已做**：加 `Path *string`（nil=非 mount，纯联邦读用法不受影响；非 nil 才校验）、`SubPath`；`DefineWorkspace` 校验「一旦有一条声明 Path 则全部必须声明」与「路径不可重复/嵌套，根不算嵌套」 |
| ~~3~~ | ~~无写回路由~~ | 路径反查落点 | **已做**：`catalog.RouteMount` / `RouteMounts`；`kc commit --workspace` 对 CheckoutMounts 树上的脏文件按仓拆成 N 次 `Writer.RawWrite`，`command-id` 按仓加后缀，第二个失败不回滚第一个。无写权的脏 mount 整次挡住。见 `catalog/checkout.go` `CollectMountChanges`、`cli/consume.go` `commitWorkspace` |
| ~~4~~ | ~~`repo-add` 的 `filegit`/`dolt` 目录焊死在 `layout.repos/<encoded-id>`~~ | 能指向已有本地 git 仓 | **已做**：`repo-add` / `kc mount --dir <git>` 写 `.kc-link` 指针、`local.AttachGit` 打开、不 stamp、不建 `streams/`；`--link` clone 进 layout.repos（gitea `--link` 即 `--dsn`）；filegit 的文件系统 `--dsn` 当作 `--dir` |
| ~~5~~ | ~~`reader.WriteCheckout` 只有 ② 的检出~~ | 本层的文件树检出 + 可写树 | **已做**（见原第 5 步长注）。写回决策已定：Loom 层走 `RAW_WRITE`（不解释 frontmatter）；知识 PUT 仍是 ② 的 `--repo`。`kc checkout --workspace` 遇 mount 配方走 `CheckoutMounts`（`--to` 可选），联邦读配方仍走 `reader.WriteCheckout`。`kc sync` 接 `SyncMounts`。`kc status --workspace/--to` 接 `MountStatus` |
| ~~6~~ | ~~checkout 不按 `--as` 裁剪~~ | agent 边界在 checkout 时 | **已做**：`CheckoutMountsAllowing`；无权的 mount `Skipped` 且不落盘；agent 树与自己的树是两次 `--to` |
| ~~7~~ | ~~`HashResolved` 只哈希 `workspaceID` + `{仓→commit}`~~ | 凡决定读结果的都参与 | **已做**：路径布局 + AppendCuts 进入 `PinID`；`revision` 不参与 |
| ~~8~~ | ~~`PinID` 只在 controlplane 用~~ | 通用可复现引用 | **已做**：`ResolvedWorkspace.PinID`；`kc resolve --workspace` 带出；消费命令 `--pin <ResolvedWorkspace.json>` 重放（仍逐成员求值 allow）。PreviewId 用同一个 PinID |
| ~~9~~ | ~~checkout 是整树替换~~ | 每条 mount 独立同步 | **已做**：`SyncMounts` / `kc sync` |
| ~~10~~ | ~~`DumpState` 不按 `--as` 裁剪~~ | 无权与不存在不可区分 | **已做**：`kc read --catalog` / `inspect` 逐仓过 allow，整份配方都看不见的 Workspace 一并省略 |
| ~~11~~ | ~~配方只在 Catalog 登记表~~ | 仓根 `.kc-workspace.yaml` 跟着 git 走 | **已做**：`catalog.WorkspaceRecipe`；`define-workspace` 对 mount 配方把 yaml 写进根仓（无根则落 `<home>/workspaces/`）；`--file` / `--from-repo` 导入；消费命令在 Catalog 没有这条配方时扫描已挂成员的 yaml 并 `DefineWorkspace`。bob `git clone` 只拿到根仓内容，织出来的 mount 不在那个 clone 里 |
| ~~12~~ | ~~组合语汇不统一~~ | Workspace 是唯一公开概念 | **已做**：类型 `WorkspaceDefinition` / `ResolveWorkspace` / `WorkspacePin`；JSON `workspaceId`；登记表 `workspace-*.yaml`；CLI 只接受 `--workspace` / `define-workspace` / `retire-workspace`；错误码是 `WORKSPACE_INVALID`，不读取旧格式 |
| ~~13~~ | ~~无本机叠加层~~ | `local_manifests` | **已做**：`.kc/overlays/<principal>/<workspace>.yaml`；`catalog.MergeOverlay`（add/replace/remove）；`kc overlay --file/--clear`；只对本机、这个 `--as`；不改共享配方、不进 hitchhiking yaml |
| ~~14~~ | ~~无配方层 CAS~~ | `base-rev` | **已做**：`WorkspaceSource.BaseRev`；selector 尖端不是该 commit 则 `NON_FAST_FORWARD`；yaml `baseRev` 与 `kc define-workspace --base-rev repo=commit` |

依赖顺序 1→2→3→5/6 已走完，§1.4 的例子能真跑通。配方文件跟着 git 走。Workspace 语汇已统一。本机叠加层与配方层 CAS 已接通。

第 1 步落地时确认的一条：能力缺失要在**用到的地方**报，不在挂载时预先拦。裸仓挂得进、组合得了、commit 得进去，只有真去拼装对象值、建索引或声明 `schema_ref` 时才撞墙，且错误直说「这个仓是普通快照，不解释知识文件」。这就是 §1.3 采用阶梯在类型上的形状。

第 2/3 步落地时确认的一条：`Path` 用 `*string` 而不是 `string`，是为了让「没声明」和「声明为根」在类型上可分——两者都合法但含义不同，前者是纯联邦读、不受任何 mount 校验约束；后者是「新文件的兜底」，参与路径唯一性检查但天然不与任何嵌套路径冲突。`RouteMount` 只吃 `WorkspaceDefinition`（配方），不吃 `ResolvedWorkspace`（这次坐标）：一个文件属于哪个仓是配方的静态属性，跟这次解到哪个 commit 无关，这正是 §2.3 里「落点由路径决定，身份由内容决定」在函数签名上的体现。

第 5 步验收时确认的一条：一个 mount 缺本机 git 目录不该拖垮整个检出——alice 的例子本来就是「自己的仓可写 + 组织政策仓只读」混着来，混合引擎（本机 + 远程 Gitea）是常见的真实形状，`CheckoutMounts` 的 Skipped 分支不是给测试凑的分支，是这条场景要求的行为。

第 5 步写回落地时确认的一条：Loom 层的写回是 `Writer.RawWrite`（字面路径、字面字节），不是从 frontmatter 反解 PUT，也不是在 worktree 里直接 `git commit`。① 不认 `object_id`，裸仓没有 Address；知识 PUT 仍走 ② 的 `--repo`。`MountStatus` / `CollectMountChanges` 仍是读原语，CLI 才决定要不要变成一次 RawWrite。
