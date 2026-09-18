# 走查

定位：整理稿。guide。可执行步骤在 [`.data/scenes/`](../../.data/scenes/README.md)。现稿 [`WALKTHROUGH_v5.1.md`](../WALKTHROUGH_v5.1.md) 仍有效，直到本篇升格。

---

## 1. 定位、边界、规范

**定位。** 走查写「操作 → 进入的状态」。读者从一个已经构建好的前态出发，做进入下一状态的那一步，再看后态。章节是给人走的任务，不是命令课，也不是覆盖清单。

没有走查，协议旅程只存在于场景树和测试名里，接入方看不到「现在在哪、下一步改什么」。

**边界。**

- 不定义协议、错误码、不变量。那是设计文档和 `ARCHITECTURE_INVARIANTS.md`。
- 不定 argv / HTTP 形状。那是 `cli/surface.go` 与 Conformance。
- 不当覆盖格子或运行证据。那是 `TEST_CATALOG.md`。
- 不教怎么写 feature、怎么嵌目录、执行器怎么跑。那是 `.data/scenes/README.md`。
- 不拥有 U1–U10 应然旅程。那是 `KNOWLEDGE_PRODUCT_AND_SCHEMA.md`。
- 不写数仓实体、容量、产品手册。
- 不套设计文档五段合同。guide 只写目标、前提、操作、可观察结果。

**规范。** 走查服从场景树的构建法，不另编一条「真人 CLI 故事」。

1. 正文按 `.data/scenes/` 的状态目录树维护。目录嵌套 = 构建前置。用例本身住在目录里（`_meta.yaml`、construct、probe、入口上的 `_bundles.yaml`），不抄进中央清单。
2. 从树上已有节点起笔。不从空 Home 把祖先重写一遍，不把部署方 setup 插进消费者任务。
3. 进入状态的步骤必须能在该目录的 `_build/construct.feature`（或登记的 go-test）找到。观测钉七列或错误码，禁止只写命令成功。
4. 树父不是发权。attach / Workspace / pin 都不授 `knowledge.read`。
5. create 与 attach 并列；`knowledge-published` 与 `domain-schema-published` 是两条写入脊，不得合成一节。
6. `_build` / `_materials` / `_probes` / `_results` 不是分叉。整体视图用 `python3 .data/scenes/tree.py` 从目录抽出。
7. 公开命令闭集由场景 construct / probe 上的测试钉住，不另写一份覆盖清单。
8. 只用这棵协议树。不出现已删除的 `.data/data-warehouse` 或 `warehouse-agent`。走查叶夹具是清河茶铺，写在 `named-repositories-created`。

独立验收：走查里的树与 `.data/scenes/` 状态目录一一对应。失败：login 当树根；create 写成 attach 的父节点；线性命令 dump 冒充走查。

---

## 2. 目录树

与 [`.data/scenes/`](../../.data/scenes/) 同一棵树。子目录是后继状态；注释是走查说明，不是第二份协议。

```text
.data/scenes/
  catalog-initialized/                         # 登记表出生。bootstrap 从这里看库存
    schema-browsed/                            # Schema 分页目录；不是对象 LIST
    http-served/                               # 测试 Server。http-local：登录配对
      access-audited/                          # access/trace/hitmap；不是调试日志
    absent-product-surfaces/                   # LIST / connector-run / APPEND / MCP 必须拒绝
    catalog-allow-ready/                       # Catalog 授权面已就绪（空规则）。catalog-allow / catalog-audit 入口
      grants-bootstrapped/                     # 首次部署的管理主体；旧本机 bootstrap 不能覆盖
      catalog-read-granted/                    # catalog.read 可见本 Catalog 库存；不放行审计/发权
        grant-revoked/                         # 撤权后旧 pin 仍按当前权限检查
      catalog-audit-granted/                   # catalog.audit.read 可读登记表历史；不放行库存
      catalog-create-granted/                  # catalog.repositories.create 是建仓准入；不放行库存/发权
      catalog-declared-private/                # 私有 Catalog 仍要 catalog.read；go-test
    managed-repository-created/                # 平台 create。与 attach 并列；go-test，无 construct
    named-repositories-created/                # 走查：create+attach table-meta、sales-semantic
      access-runtime-ready/                    # 走查：两仓用例知识 + table-meta 接入方容器
        index-ready/                           # 投影追上；SEARCH
          sales-dataset-defined/               # 叶：两仓表层对象 → qinghe-sales。goto.py
    deployment-restored/                       # 替换实例缓存后恢复。重新 init 不算恢复；go-test
    system-schema-published/                   # System Schema 可读；禁止 Writer PUT
      repository-attached/                     # 既有仓只读验证并原子登记。接入完成。product-core 入口
        catalog-inventory-visible/             # 已接入成员仓身份可见；不放行正文、README
        repository-declared-readable/          # 仓已认证默认可读；未声明 fail closed；go-test
        writer-granted/                        # 接入方可以 COMMIT；旁观者写失败
        catalog-archived/                      # 禁 define；不是删知识
        repository-archived/                   # 该仓离开本 Catalog；Snapshot 对象仍在
        changeset-previewed/                   # Collector Preview；runtime 墙外
        domain-schema-published/               # writer commit --dir 发布 Domain Schema
            schema-read-granted/               # 可 browse/describe；不放行实例正文
            access-handle-published/           # Binding 句柄进仓；当前值不进 Snapshot
              observation-refreshed/           # 动态观察。HEAD 不动；不进 product 套件
              resource-access-granted/         # hydrate 墙外资源；要落知识仍走 Writer
            semantic-knowledge-constructed/    # 按已发布 Schema PUT 实例
              projection-synced/               # 投影追上 HEAD。P-22 入口；SEARCH 前提，不是 READ 前提
                knowledge-search-granted/      # 搜宽：可定位
                  knowledge-read-granted/      # 读严：正文按仓读权交付
                  retrieval-refined/           # 同一 SearchView 上 rerank；不改 Canonical
              knowledge-set-defined/           # 语义脊上的命名知识集 scene-set
                principals-granted/            # P-23：同一消费者发现 → pin → SEARCH → READ
                dataset-consume-granted/          # 可进知识集；不放行 knowledge.*
                dataset-manage-granted/           # 可发布/退役该 Dataset；不放行 file.read
                dataset-resolve-granted/          # 可 pin；不发权、不读正文
                dataset-retired/                  # 知识集不可 Open；成员仓仍在
                file-view-planned/             # 只读文件投影计划；pin 不随 HEAD 动
                  dataset-mounted/                # kcfs 挂载。不是 Writer
        knowledge-published/                   # 普通 Canonical PUT。另一条写入脊
          permissions-aspect-published/        # permissions Aspect 是知识，不是 kc 闸门
          dataset-defined/                        # Canonical 脊上的知识集 scene-notes。maintenance 入口
            dataset-federated/                    # 同 object_id 两仓并存，不按 scope 覆盖
            proposal-opened/                   # candidate 在；main 不动
              proposal-previewed/
                proposal-validated/
                  validation-recorded/
                    proposal-merged/           # 只推进目标仓 Ref；下次重新 pin 才见新值
```

树上有、但不在下列任务里的节点（`create`、恢复、动态观察、挂载等），从该目录自己走，不另开章。

---

## 3. 任务

给人走的章节 = 入口节点上的 `_bundles.yaml`。从该前态起，按 walk 一次 construct 进入下一状态；停在节点上的 probe 是另一种风险，不是新章。命令在对应目录的 construct，这里不抄 argv。

### bootstrap · `catalog-initialized`

**目标。** 登记表已经出生，能看见库存前态。

**前提。** 部署夹具已给出空 Catalog 与 System 信任根。

**操作。** 看当前 Catalog。

**结果。** `catalogId` 是 `kr://scene/catalog`，Workspace 空，System 仓已在库存。没有 `home` / `namespace`。不发 `knowledge.read`。

### catalog-allow · `catalog-allow-ready`

**目标。** 给已认证主体 `catalog.read`，并证明它不是审计或发权。

**前提。** Catalog 授权面已就绪（空规则）。本机 Home 无 Deployment，公开 Catalog 对 `--as` 仍要 grant。

**操作 → 状态。**

```text
catalog-read-granted       可见本 Catalog 库存身份；不放行审计、发权、归档
grant-revoked              撤权立即生效；旧 pin 不能绕过
```

### catalog-audit · `catalog-allow-ready`

**目标。** `catalog.audit.read` 可读登记表历史，不放行库存发现。

**前提。** Catalog 授权面已就绪（空规则）。

**操作 → 状态。** `catalog-audit-granted`。

### catalog-create · `catalog-allow-ready`

**目标。** `catalog.repositories.create` 是建仓准入，不是库存或发权。

**前提。** Catalog 授权面已就绪（空规则）。本机 Home 无 Deployment。

**操作 → 状态。** `catalog-create-granted`。规则可被 `grant list` 求值命中；不放行库存或发权。真正建仓走 `managed-repository-created`。

### product-core · `repository-attached`

**目标。** 接入完成后发表、读 Schema、归档。

**前提。** 既有 Snapshot 已 attach。这是接入完成，不是 create。

**操作 → 状态。**

```text
catalog-inventory-visible  可见成员仓身份，不见正文/README
repository-declared-readable 仓已认证默认可读；go-test
writer-granted             接入方可 COMMIT；旁观者写失败
knowledge-published        Canonical 脊：普通 PUT 已发表
permissions-aspect-published  permissions 是知识，不是闸门
domain-schema-published    writer commit --dir 发布 Domain Schema
schema-read-granted        可 browse / describe；不放行实例
catalog-archived           禁 define；不是删知识
```

两条写入脊都从本入口走到，但不得并成「发布并回读」一节。

### http-local · `http-served`

**目标。** 测试 Server 上的登录绑定。

**前提。** HTTP 已起来。

**操作。** 配对登录；空凭证仍打同一 Server。

**结果。** `whoami` 绑到当前 principal。空凭证拒绝。登录不是树根，也不发权。

### absent-surfaces · `absent-product-surfaces`

**目标。** 冻结入口必须失败。

**前提。** 已有部署。

**操作。** 打对象 LIST、connector-run、APPEND、MCP、checkout/export。

**结果。** 未知命令或协议错误。没有「暂未实现」的正路径。

### P-22 · `projection-synced`

**目标。** 搜宽读严。

**前提。** 声明式索引已追上 published HEAD。投影是 SEARCH 前提，不是 READ 前提。

**操作 → 状态。**

```text
knowledge-search-granted   可定位候选；无读权则正文剥离，不标 partial
knowledge-read-granted     有仓读权才交付 Canonical
```

### P-23 · `principals-granted`

**目标。** 同一消费者自己发现、固定版本、检索、读取。

**前提。** 语义脊上已有知识集 `scene-set`，且该消费者已获权。不要把 operator 的 setup 插进来替他取材料。

**操作。** 发现知识集 → pin → SEARCH → READ。全程同一主体、经 Server。

**结果。** `file.read` 只让进知识集，不放行 `knowledge.*`。pin 不发权。无读权则命中仍在、正文剥离。

### maintenance · `dataset-defined`

**目标。** 已有知识上走完提案到合并。

**前提。** Canonical 脊上已有命名知识集。Gate 只绑 merge，COMMIT 不走 Gate。

**操作 → 状态。**

```text
proposal-opened        candidate 在；main 不动；--repo 仍旧值
proposal-previewed     overlay 到知识集 pin，得到 Preview basis
proposal-validated     协议结构检查；不跑业务套件
validation-recorded    只绑定外部套件已给出的 PASSED / FAILED
proposal-merged        只推进目标仓 Ref；下次重新 pin 才见新值
```
