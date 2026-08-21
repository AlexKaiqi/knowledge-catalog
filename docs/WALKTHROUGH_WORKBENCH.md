# 公司工作台走通（三仓 · 两 View）

用 `kc` **逐步实跑**立项拆法：平台对源负责、组织对口径负责、个人是后置发表。每一步先给命令，再贴 **这一步真实观测**（退出码、stdout、当时的文件/git）。哈希是这一次 `--home /tmp/kc-workbench` 的结果；你重跑会得到另一组 SHA，相对关系应相同。

对照：`[scenario/README.md](../scenario/README.md)`（Go API 套件）；`[WALKTHROUGH_v5.1.md](WALKTHROUGH_v5.1.md)`；`[PERMISSIONS.md](PERMISSIONS.md)`。

```bash
export PATH="$HOME/.local/go/bin:$PATH"
export KC_HOME=/tmp/kc-workbench
kc() { go run ./cmd/kc -- --home "$KC_HOME" "$@"; }

# 本走通的 changeset JSON 在 /tmp/kc-wb-changesets（不是协议对象）
```

```text

Catalog  kr://acme/catalog
├── kr://acme/public/metadata     平台统一元数据；采集 COMMIT
├── kr://acme/org/semantics       组织口径 + 例题；认领走 PROPOSAL
└── kr://acme/personals/kai       个人习惯 / 问题分布；COMMIT + APPEND
```


| View            | 成员                   | Release     | 谁跟       |
| --------------- | -------------------- | ----------- | -------- |
| `analyst-board` | metadata + semantics | 公司 `stable` | 分析 Agent |
| `kai-desk`      | 仅 personal           | 个人 `desk`   | 个人 Agent |


看四列：成员库 `main`、候选 Ref、Catalog 当前态 `read --catalog`、读者 `read --release`。

### 本跑短名

从下面各步 stdout / git 回填。你重跑会得到另一组 SHA。


| 111短名     | 值                                                                          |
| ---------- | -------------------------------------------------------------------------- |
| `Meta0`    | `396d4d12ab540816ae90b8b579f596c237eaaf40`                                 |
| `Sem0`     | `396d4d12ab540816ae90b8b579f596c237eaaf40`                                 |
| `Kai0`     | `396d4d12ab540816ae90b8b579f596c237eaaf40`                                 |
| `U1`       | `b40f0ddf703ded326109f2fdf56ff265d0d9009b`                                 |
| `S1`       | `b82d56897213444f52f655c28261986a5ba02d13`                                 |
| `K1`       | `7b0056ae331eefbbf000695bda059c9e958bf322`                                 |
| `G1`       | `f5daa2aac586704da4a0c64335adf71f9340fc435cd131cf762f79f4fd30c715`         |
| `G1_meta`  | `b40f0ddf703ded326109f2fdf56ff265d0d9009b`                                 |
| `G1_sem`   | `b82d56897213444f52f655c28261986a5ba02d13`                                 |
| `Cpv`      | `40ab9f1e5d5a72c880c28136feb41a7129f41412`                                 |
| `Gpv`      | `90b27ad10a46b3b9be0357e5ca6dbba4678d9ce109fff4ed4e2e40a359eb4781`         |
| `preview`  | `preview-90b27ad10a46b3b9be0357e5ca6dbba4678d9ce109fff4ed4e2e40a359eb4781` |
| `C1`       | `40ab9f1e5d5a72c880c28136feb41a7129f41412`                                 |
| `G2`       | `90b27ad10a46b3b9be0357e5ca6dbba4678d9ce109fff4ed4e2e40a359eb4781`         |
| `Gdesk`    | `09956d653ae5304ffdac92d7d8c8ca56234be4ac2746d0baf0a91161000a0dbd`         |
| `K2`       | `8dae467790626629939a73b0992fd7be4753e750`                                 |
| `Goverlay` | `dcdc2c8e204e8b567b418e586e60ae77bda82188eb284f91cf3c1dda8f267285`         |
| `S2`       | `727c859dd036edd76f39fdac428cade2de533f43`                                 |
| `U2`       | `ba9d0cc8a6644b5f795b34a7fa2875f2a47a4761`                                 |


## S0　空 Catalog + 三仓 + 角色

### S0.1 init

**操作**

```bash
kc init --catalog acme/catalog
```

**观测结果**

- 退出码：`0`
- stdout：

```json
{
  "catalog": "kr://acme/catalog"
}
```

这一步只指名第一间 Catalog。没有知识、没有 View、没有 Release。`created` / `initialized` 不回显——那不是访问。

当前组合空间（成员、配方、代、发布指针）看 `kc read --catalog`，不是知识 `READ`，也不是本机 `status`：

```bash
kc read --catalog
```

**观测结果**

- 退出码：`0`
- stdout：

```json
{
  "views": [],
  "generations": [],
  "releases": {},
  "repositories": [],
  "catalogId": "kr://acme/catalog"
}
```

空：没有已登记的仓、没有 View、没有 Release。

登记表 git 是改动历史，不是当前值：

```bash
kc audit
```

**观测结果**

- 退出码：`0`
- stdout：

```json
{
  "catalogId": "kr://acme/catalog",
  "entries": [
    {
      "author": "knowledge-catalog",
      "commit": "ab52bedadf48e0a0f133bcb64590cd84623756a3",
      "message": "init kr://acme/catalog"
    },
    {
      "author": "knowledge-catalog",
      "commit": "396d4d12ab540816ae90b8b579f596c237eaaf40",
      "message": "root"
    }
  ],
  "source": "catalog"
}
```

`kc status` 是本机扫到哪些仓/配方，混 stores，不是 Catalog 正文。

### S0.1c 消费访问：还没有 Release

**操作**

```bash
kc read --release stable --object Table:dwd.trade_order
```

**观测结果**

- 退出码：`1`
- stdout：

```json
{
  "error": {
    "code": "VIEW_GENERATION_INVALID",
    "message": "unknown release stable"
  }
}
```

符合预期：还没有 Release。

### S0.1d 不能把 Catalog id 当成员仓

**操作**

```bash
kc repo-add --repo kr://acme/catalog
```

**观测结果**

- 退出码：`1`
- stdout：

```json
{
  "error": {
    "message": "kr://acme/catalog is reserved for a Catalog registry"
  }
}
```

符合预期：登记表不是知识仓。

### S0.2 repo-add kr://acme/public/metadata

**操作**

```bash
kc repo-add --repo kr://acme/public/metadata
```

**观测结果**

- 退出码：`0`
- stdout：

```json
{
  "head": "396d4d12ab540816ae90b8b579f596c237eaaf40",
  "repositoryId": "kr://acme/public/metadata"
}
```

### S0.2 repo-add kr://acme/org/semantics

**操作**

```bash
kc repo-add --repo kr://acme/org/semantics
```

**观测结果**

- 退出码：`0`
- stdout：

```json
{
  "head": "396d4d12ab540816ae90b8b579f596c237eaaf40",
  "repositoryId": "kr://acme/org/semantics"
}
```

### S0.2 repo-add kr://acme/personals/kai

**操作**

```bash
kc repo-add --repo kr://acme/personals/kai
```

**观测结果**

- 退出码：`0`
- stdout：

```json
{
  "head": "396d4d12ab540816ae90b8b579f596c237eaaf40",
  "repositoryId": "kr://acme/personals/kai"
}
```

### S0.2 之后的仓库文件

- 登记表文件

```text
catalog.yaml
repository-kr_acme_org_semantics.yaml
repository-kr_acme_personals_kai.yaml
repository-kr_acme_public_metadata.yaml
```

- 仓登记 yaml

```yaml
repository: kr://acme/org/semantics

---
repository: kr://acme/personals/kai

---
repository: kr://acme/public/metadata
```

- 登记表 git

```text
33059b5 register kr://acme/personals/kai
58725c2 register kr://acme/org/semantics
9ab2976 register kr://acme/public/metadata
ab52bed init kr://acme/catalog
396d4d1 root
```

- 成员仓 identity / 空 tree

```text
metadata kr://acme/public/metadata HEAD=396d4d12ab540816ae90b8b579f596c237eaaf40
（空 tree）

semantics kr://acme/org/semantics HEAD=396d4d12ab540816ae90b8b579f596c237eaaf40
（空 tree）

personal kr://acme/personals/kai HEAD=396d4d12ab540816ae90b8b579f596c237eaaf40
（空 tree）
```

### S0.3 allow --principal collector --cmd put,commit --repo kr://acme/public/metadata

**操作**

```bash
kc allow --principal collector --cmd put,commit --repo kr://acme/public/metadata
```

**观测结果**

- 退出码：`0`
- stdout：

```json
{
  "id": "alw_1",
  "principal": "collector",
  "cmds": [
    "put",
    "commit"
  ],
  "repo": "kr://acme/public/metadata"
}
```

### S0.3 allow --principal steward --cmd propose --repo kr://acme/org/semantics

**操作**

```bash
kc allow --principal steward --cmd propose --repo kr://acme/org/semantics
```

**观测结果**

- 退出码：`0`
- stdout：

```json
{
  "id": "alw_2",
  "principal": "steward",
  "cmds": [
    "propose"
  ],
  "repo": "kr://acme/org/semantics"
}
```

### S0.3 allow --principal steward --cmd merge --repo kr://acme/org/semantics

**操作**

```bash
kc allow --principal steward --cmd merge --repo kr://acme/org/semantics
```

**观测结果**

- 退出码：`0`
- stdout：

```json
{
  "id": "alw_3",
  "principal": "steward",
  "cmds": [
    "merge"
  ],
  "repo": "kr://acme/org/semantics"
}
```

### S0.3 allow --principal steward --cmd define-view,pin-view,promote --catalog kr://acme/catalog

**操作**

```bash
kc allow --principal steward --cmd define-view,pin-view,promote --catalog kr://acme/catalog
```

**观测结果**

- 退出码：`0`
- stdout：

```json
{
  "id": "alw_4",
  "principal": "steward",
  "cmds": [
    "define-view",
    "pin-view",
    "promote"
  ],
  "catalog": "kr://acme/catalog"
}
```

### S0.3 allow --principal steward --cmd preview,validate,record-validation --catalog kr://acme/catalog

**操作**

```bash
kc allow --principal steward --cmd preview,validate,record-validation --catalog kr://acme/catalog
```

**观测结果**

- 退出码：`0`
- stdout：

```json
{
  "id": "alw_5",
  "principal": "steward",
  "cmds": [
    "preview",
    "validate",
    "record-validation"
  ],
  "catalog": "kr://acme/catalog"
}
```

### S0.3 allow --principal steward --cmd rollback,retire-view,retire-release,index-plan,archive-catalog --catalog kr://acme/catalog

**操作**

```bash
kc allow --principal steward --cmd rollback,retire-view,retire-release,index-plan,archive-catalog --catalog kr://acme/catalog
```

**观测结果**

- 退出码：`0`
- stdout：

```json
{
  "id": "alw_6",
  "principal": "steward",
  "cmds": [
    "rollback",
    "retire-view",
    "retire-release",
    "index-plan",
    "archive-catalog"
  ],
  "catalog": "kr://acme/catalog"
}
```

### S0.3 allow --principal kai --cmd put,commit --repo kr://acme/personals/kai

**操作**

```bash
kc allow --principal kai --cmd put,commit --repo kr://acme/personals/kai
```

**观测结果**

- 退出码：`0`
- stdout：

```json
{
  "id": "alw_7",
  "principal": "kai",
  "cmds": [
    "put",
    "commit"
  ],
  "repo": "kr://acme/personals/kai"
}
```

### S0.3 allow --principal kai --cmd append --repo kr://acme/personals/kai --stream practice

**操作**

```bash
kc allow --principal kai --cmd append --repo kr://acme/personals/kai --stream practice
```

**观测结果**

- 退出码：`0`
- stdout：

```json
{
  "id": "alw_8",
  "principal": "kai",
  "cmds": [
    "append"
  ],
  "repo": "kr://acme/personals/kai",
  "stream": "practice"
}
```

### S0.3 allow --principal kai --cmd read --repo kr://acme/personals/kai

**操作**

```bash
kc allow --principal kai --cmd read --repo kr://acme/personals/kai
```

**观测结果**

- 退出码：`0`
- stdout：

```json
{
  "id": "alw_9",
  "principal": "kai",
  "cmds": [
    "read"
  ],
  "repo": "kr://acme/personals/kai"
}
```

### S0.3 allow --principal kai --cmd read-release --catalog kr://acme/catalog --release desk

**操作**

```bash
kc allow --principal kai --cmd read-release --catalog kr://acme/catalog --release desk
```

**观测结果**

- 退出码：`0`
- stdout：

```json
{
  "id": "alw_10",
  "principal": "kai",
  "cmds": [
    "read-release"
  ],
  "catalog": "kr://acme/catalog",
  "release": "desk"
}
```

### S0.3 allow --principal analyst-agent --cmd read-release --catalog kr://acme/catalog --release stable

**操作**

```bash
kc allow --principal analyst-agent --cmd read-release --catalog kr://acme/catalog --release stable
```

**观测结果**

- 退出码：`0`
- stdout：

```json
{
  "id": "alw_11",
  "principal": "analyst-agent",
  "cmds": [
    "read-release"
  ],
  "catalog": "kr://acme/catalog",
  "release": "stable"
}
```

### S0.3 allow --principal analyst-agent --cmd read --repo kr://acme/public/metadata

**操作**

```bash
kc allow --principal analyst-agent --cmd read --repo kr://acme/public/metadata
```

**观测结果**

- 退出码：`0`
- stdout：

```json
{
  "id": "alw_12",
  "principal": "analyst-agent",
  "cmds": [
    "read"
  ],
  "repo": "kr://acme/public/metadata"
}
```

### S0.3 allow --principal analyst-agent --cmd read --repo kr://acme/org/semantics

**操作**

```bash
kc allow --principal analyst-agent --cmd read --repo kr://acme/org/semantics
```

**观测结果**

- 退出码：`0`
- stdout：

```json
{
  "id": "alw_13",
  "principal": "analyst-agent",
  "cmds": [
    "read"
  ],
  "repo": "kr://acme/org/semantics"
}
```

### S0.3 gate-add --on merge --repo kr://acme/org/semantics --require validate,suite:steward

**操作**

```bash
kc gate-add --on merge --repo kr://acme/org/semantics --require validate,suite:steward
```

**观测结果**

- 退出码：`0`
- stdout：

```json
{
  "id": "gt_1",
  "on": "merge",
  "repo": "kr://acme/org/semantics",
  "require": [
    "validate",
    "suite:steward"
  ]
}
```

### S0.3 之后的 allow / gate 文件

- allow.json

```json
{
  "rules": [
    {
      "id": "alw_1",
      "principal": "collector",
      "cmds": [
        "put",
        "commit"
      ],
      "repo": "kr://acme/public/metadata"
    },
    {
      "id": "alw_2",
      "principal": "steward",
      "cmds": [
        "propose"
      ],
      "repo": "kr://acme/org/semantics"
    },
    {
      "id": "alw_3",
      "principal": "steward",
      "cmds": [
        "merge"
      ],
      "repo": "kr://acme/org/semantics"
    },
    {
      "id": "alw_4",
      "principal": "steward",
      "cmds": [
        "define-view",
        "pin-view",
        "promote"
      ],
      "catalog": "kr://acme/catalog"
    },
    {
      "id": "alw_5",
      "principal": "steward",
      "cmds": [
        "preview",
        "validate",
        "record-validation"
      ],
      "catalog": "kr://acme/catalog"
    },
    {
      "id": "alw_6",
      "principal": "steward",
      "cmds": [
        "rollback",
        "retire-view",
        "retire-release",
        "index-plan",
        "archive-catalog"
      ],
      "catalog": "kr://acme/catalog"
    },
    {
      "id": "alw_7",
      "principal": "kai",
      "cmds": [
        "put",
        "commit"
      ],
      "repo": "kr://acme/personals/kai"
    },
    {
      "id": "alw_8",
      "principal": "kai",
      "cmds": [
        "append"
      ],
      "repo": "kr://acme/personals/kai",
      "stream": "practice"
    },
    {
      "id": "alw_9",
      "principal": "kai",
      "cmds": [
        "read"
      ],
      "repo": "kr://acme/personals/kai"
    },
    {
      "id": "alw_10",
      "principal": "kai",
      "cmds": [
        "read-release"
      ],
      "catalog": "kr://acme/catalog",
      "release": "desk"
    },
    {
      "id": "alw_11",
      "principal": "analyst-agent",
      "cmds": [
        "read-release"
      ],
      "catalog": "kr://acme/catalog",
      "release": "stable"
    },
    {
      "id": "alw_12",
      "principal": "analyst-agent",
      "cmds": [
        "read"
      ],
      "repo": "kr://acme/public/metadata"
    },
    {
      "id": "alw_13",
      "principal": "analyst-agent",
      "cmds": [
        "read"
      ],
      "repo": "kr://acme/org/semantics"
    }
  ]
}
```

- gates.json

```json
{
  "rules": [
    {
      "id": "gt_1",
      "on": "merge",
      "repo": "kr://acme/org/semantics",
      "require": [
        "validate",
        "suite:steward"
      ]
    }
  ]
}
```

- 登记表 git（应仍只有 init+register）

```text
33059b5 register kr://acme/personals/kai
58725c2 register kr://acme/org/semantics
9ab2976 register kr://acme/public/metadata
ab52bed init kr://acme/catalog
396d4d1 root
```

### S0.4 未登记仓不能进配方

**操作**

```bash
kc define-view --as steward --view analyst-board --revision 1 --source kr://acme/unknown/ghost=refs/heads/main
```

**观测结果**

- 退出码：`1`
- stdout：

```json
{
  "error": {
    "code": "VIEW_GENERATION_INVALID",
    "message": "repository kr://acme/unknown/ghost is not registered in this catalog"
  }
}
```

当时磁盘 / 其它命令：

- 登记表仍无 view-*.yaml

```text
catalog.yaml
repository-kr_acme_org_semantics.yaml
repository-kr_acme_personals_kai.yaml
repository-kr_acme_public_metadata.yaml
```

符合预期：`VIEW_GENERATION_INVALID`。

### S0.5 采集员不能写口径仓

**操作**

```bash
kc put --as collector --command-id steal --repo kr://acme/org/semantics --object Metric:gmv --aspect definition --value '{"formula":"no"}'
```

**观测结果**

- 退出码：`1`
- stdout：

```json
{
  "error": {
    "code": "FORBIDDEN",
    "message": "collector is not allowed to put"
  }
}
```

符合预期：`FORBIDDEN`。

### S0.6 分析 Agent 不能写

**操作**

```bash
kc put --as analyst-agent --command-id agent-write --repo kr://acme/public/metadata --object Table:x --value '{"v":1}'
```

**观测结果**

- 退出码：`1`
- stdout：

```json
{
  "error": {
    "code": "FORBIDDEN",
    "message": "analyst-agent is not allowed to put"
  }
}
```

符合预期：`FORBIDDEN`。

## S1　写入三仓，Catalog 不动

### S1.1 坏 schema_ref

**操作**

```bash
kc put --as collector --command-id bad-schema --repo kr://acme/public/metadata --object Table:dwd.trade_order --aspect structure --schema-ref schema/missing --value '{"db":"dw"}'
```

**观测结果**

- 退出码：`1`
- stdout：

```json
{
  "error": {
    "code": "SCHEMA_REVISION_UNRESOLVED",
    "message": "schema_ref \"schema/missing\" is unresolved"
  }
}
```

当时磁盘 / 其它命令：

- metadata HEAD 仍是空仓

```text
396d4d12ab540816ae90b8b579f596c237eaaf40
（空 tree）
```

符合预期：`SCHEMA_REVISION_UNRESOLVED`，无部分文件。

### S1.2 DERIVATION 缺算法

**操作**

```bash
kc put --command-id bad-derivation --repo kr://acme/org/semantics --object note/derived --value '{"v":1}' --origin-kind DERIVATION
```

**观测结果**

- 退出码：`1`
- stdout：

```json
{
  "error": {
    "code": "PRECONDITION_FAILED",
    "message": "DERIVATION provenance requires a fixed input ViewReadVersion and algorithm identity"
  }
}
```

符合预期：`PRECONDITION_FAILED`。

### S1.3 采集员不能 APPEND

**操作**

```bash
kc append --as collector --command-id append-meta --repo kr://acme/public/metadata --stream practice --event-id evt-meta --payload '{"note":"no"}'
```

**观测结果**

- 退出码：`1`
- stdout：

```json
{
  "error": {
    "code": "FORBIDDEN",
    "message": "collector is not allowed to append"
  }
}
```

符合预期：allow 没有 append，`FORBIDDEN`（在 Stream 是否挂载之前就被拦住）。

### S1.4 采集 COMMIT（changeset + --repo 才能过 allow）

**操作**

```bash
kc commit --as collector --request-id s1-meta --command-id meta-u1 --repo kr://acme/public/metadata --changeset /tmp/kc-wb-changesets/meta-u1.json
```

**观测结果**

- 退出码：`0`
- stdout：

```json
{
  "receiptRef": "receipt:commit:b40f0ddf703ded326109f2fdf56ff265d0d9009b",
  "commandId": "meta-u1",
  "surface": "COMMIT",
  "disposition": "APPLIED",
  "result": {
    "repositoryId": "kr://acme/public/metadata",
    "commitId": "b40f0ddf703ded326109f2fdf56ff265d0d9009b",
    "targetRef": "refs/heads/main",
    "oldCommit": "396d4d12ab540816ae90b8b579f596c237eaaf40",
    "newCommit": "b40f0ddf703ded326109f2fdf56ff265d0d9009b"
  }
}
```

当时磁盘 / 其它命令：

- git log

```text
b40f0dd meta-u1 collector boot
396d4d1 root
```

- ls-tree HEAD

```text
100644 blob 62a7a678b99f04fc981a5c75edab2c5150bc1203	objects/schema/table.ownership.json
100644 blob 58b13b9e0557d4c8944c20a9b4f742e8e2cb3df7	objects/schema/table.structure.json
100644 blob fb77af3bcb8bac249a528e7a5d99d017e518ecd6	tables/dwd.trade_order.ownership.json
100644 blob a1a628ebca88d999dcfcae045fcd243d151fd6c2	tables/dwd.trade_order.structure.json
```

- tables/dwd.trade_order.structure.json

```text
---
object_id: Table:dwd.trade_order
aspect_name: structure
kind: Aspect
path_hint: tables/dwd.trade_order.structure.json
schema_ref: schema/table.structure
provenance: {"originKind":"SOURCE","actorRef":"collector","sourceRefs":["metastore"]}
---
{
  "db": "dw",
  "name": "dwd.trade_order"
}
```

- tables/dwd.trade_order.ownership.json

```text
---
object_id: Table:dwd.trade_order
aspect_name: ownership
kind: Aspect
path_hint: tables/dwd.trade_order.ownership.json
schema_ref: schema/table.ownership
provenance: {"originKind":"SOURCE","actorRef":"collector","sourceRefs":["metastore"]}
---
{
  "owner": "platform"
}
```

- objects/schema/table.structure.json

```text
---
object_id: schema/table.structure
provenance: {"originKind":"SOURCE","actorRef":"collector","sourceRefs":["metastore"]}
---
{
  "aspect": "structure",
  "entity": "Table",
  "fields": {
    "db": {
      "access": [
        "filter",
        "key"
      ]
    },
    "name": {
      "access": [
        "filter",
        "text"
      ]
    }
  },
  "pattern": "record"
}
```

### S1.5 同一 command_id 重放

**操作**

```bash
kc commit --as collector --command-id meta-u1 --repo kr://acme/public/metadata --changeset /tmp/kc-wb-changesets/meta-u1.json
```

**观测结果**

- 退出码：`0`
- stdout：

```json
{
  "receiptRef": "receipt:commit:b40f0ddf703ded326109f2fdf56ff265d0d9009b",
  "commandId": "meta-u1",
  "surface": "COMMIT",
  "disposition": "REPLAYED",
  "result": {
    "repositoryId": "kr://acme/public/metadata",
    "commitId": "b40f0ddf703ded326109f2fdf56ff265d0d9009b",
    "targetRef": "refs/heads/main",
    "oldCommit": "396d4d12ab540816ae90b8b579f596c237eaaf40",
    "newCommit": "b40f0ddf703ded326109f2fdf56ff265d0d9009b"
  }
}
```

符合预期：`REPLAYED`，commit 仍是 U1。

### S1.6 同 command_id 异 digest

**操作**

```bash
kc commit --as collector --command-id meta-u1 --repo kr://acme/public/metadata --changeset /tmp/kc-wb-changesets/meta-u1-conflict.json
```

**观测结果**

- 退出码：`1`
- stdout：

```json
{
  "error": {
    "code": "IDEMPOTENCY_CONFLICT",
    "message": "command meta-u1 reused with different payload"
  }
}
```

当时磁盘 / 其它命令：

- HEAD 仍是 U1

```text
b40f0ddf703ded326109f2fdf56ff265d0d9009b
```

符合预期：`IDEMPOTENCY_CONFLICT`。

### S1.7 拼装读表（维护口 --repo/--ref）

**操作**

```bash
kc read --repo kr://acme/public/metadata --object Table:dwd.trade_order --ref refs/heads/main
```

**观测结果**

- 退出码：`0`
- stdout：

```json
{
  "knowledgeRef": {
    "repository": "kr://acme/public/metadata",
    "object": "Table:dwd.trade_order"
  },
  "repository": "kr://acme/public/metadata",
  "commit": "b40f0ddf703ded326109f2fdf56ff265d0d9009b",
  "address": {
    "kind": "Entity",
    "objectId": "Table:dwd.trade_order"
  },
  "value": {
    "ownership": {
      "owner": "platform"
    },
    "structure": {
      "db": "dw",
      "name": "dwd.trade_order"
    }
  },
  "units": [
    {
      "kind": "Aspect",
      "objectId": "Table:dwd.trade_order",
      "aspectName": "ownership"
    },
    {
      "kind": "Aspect",
      "objectId": "Table:dwd.trade_order",
      "aspectName": "structure"
    }
  ]
}
```

### S1.8 口径仓种子（主人 COMMIT；认领仍走 propose）

**操作**

```bash
kc commit --command-id sem-s1 --changeset /tmp/kc-wb-changesets/sem-s1.json
```

**观测结果**

- 退出码：`0`
- stdout：

```json
{
  "receiptRef": "receipt:commit:b82d56897213444f52f655c28261986a5ba02d13",
  "commandId": "sem-s1",
  "surface": "COMMIT",
  "disposition": "APPLIED",
  "result": {
    "repositoryId": "kr://acme/org/semantics",
    "commitId": "b82d56897213444f52f655c28261986a5ba02d13",
    "targetRef": "refs/heads/main",
    "oldCommit": "396d4d12ab540816ae90b8b579f596c237eaaf40",
    "newCommit": "b82d56897213444f52f655c28261986a5ba02d13"
  }
}
```

当时磁盘 / 其它命令：

- ls-tree HEAD

```text
100644 blob 2c1852b5af1912dc6c85c601b58d50a8b1aa82bc	examples/gmv-refund.md
100644 blob b0e66908c6d56624f0738b82c4dd63b3a1762855	objects/schema/example.body.json
100644 blob a0dda6fca324259eb4f5f017cd35a8ab1e3d2b36	objects/schema/metric.definition.json
```

- examples/gmv-refund.md

```text
---
object_id: Example:gmv-refund
aspect_name: body
kind: Aspect
path_hint: examples/gmv-refund.md
schema_ref: schema/example.body
provenance: {"originKind":"DEFINITION","actorRef":"steward"}
---
{
  "prompt": "退货是否算进 GMV？"
}
```

### S1.9 Metric:gmv 此时 UNRESOLVED

**操作**

```bash
kc resolve --repo kr://acme/org/semantics --object Metric:gmv --ref refs/heads/main
```

**观测结果**

- 退出码：`0`
- stdout：

```json
{
  "repository": "kr://acme/org/semantics",
  "commit": "b82d56897213444f52f655c28261986a5ba02d13",
  "objectId": "Metric:gmv",
  "address": {
    "kind": "Entity",
    "objectId": "Metric:gmv"
  },
  "pathHint": "",
  "status": "UNRESOLVED"
}
```

符合预期：例题在，口径未认领。

### S1.10 个人仓 COMMIT

**操作**

```bash
kc commit --as kai --command-id kai-k1 --repo kr://acme/personals/kai --changeset /tmp/kc-wb-changesets/kai-k1.json
```

**观测结果**

- 退出码：`0`
- stdout：

```json
{
  "receiptRef": "receipt:commit:7b0056ae331eefbbf000695bda059c9e958bf322",
  "commandId": "kai-k1",
  "surface": "COMMIT",
  "disposition": "APPLIED",
  "result": {
    "repositoryId": "kr://acme/personals/kai",
    "commitId": "7b0056ae331eefbbf000695bda059c9e958bf322",
    "targetRef": "refs/heads/main",
    "oldCommit": "396d4d12ab540816ae90b8b579f596c237eaaf40",
    "newCommit": "7b0056ae331eefbbf000695bda059c9e958bf322"
  }
}
```

当时磁盘 / 其它命令：

- ls-tree HEAD

```text
100644 blob 72ebf2967a640007e8bf75ffb1295ccc72ab39b2	objects/Dist:error-by-topic/stats.json
100644 blob f2f7818e71d1ba380c73687df921f6af6df504a8	objects/Habit:morning-review/note.json
100644 blob cdafea631b9642d04499d4352ae5d488b8fce237	objects/schema/dist.stats.json
100644 blob 8eac79aa4ccacb42620a857d90751624e10cccec	objects/schema/habit.note.json
```

- Habit 正文

```text
---
object_id: Habit:morning-review
aspect_name: note
kind: Aspect
schema_ref: schema/habit.note
provenance: {"originKind":"OBSERVATION","actorRef":"kai"}
---
{
  "text": "每天先看昨日异常单",
  "when": "morning"
}
```

- Dist 正文

```text
---
object_id: Dist:error-by-topic
aspect_name: stats
kind: Aspect
schema_ref: schema/dist.stats
provenance: {"originKind":"OBSERVATION","actorRef":"kai"}
---
{
  "count": "12",
  "topic": "退款口径"
}
```

### S1.11 APPEND evt-1

**操作**

```bash
kc append --as kai --command-id append-evt-1 --repo kr://acme/personals/kai --stream practice --event-id evt-1 --payload '{"note":"退款口径疑问"}'
```

**观测结果**

- 退出码：`0`
- stdout：

```json
{
  "receiptRef": "receipt:append:append-evt-1",
  "commandId": "append-evt-1",
  "surface": "APPEND",
  "disposition": "APPLIED",
  "result": {
    "repositoryId": "kr://acme/personals/kai",
    "streamRef": "practice",
    "cursor": "1",
    "appended": [
      "rec-1"
    ]
  }
}
```

当时磁盘 / 其它命令：

- streams/practice.jsonl（不进 git）

```text
{"recordId":"rec-1","eventId":"evt-1","payload":{"note":"退款口径疑问"},"digest":"647aa423944464b165f58d47717814a3575b2e2d903ce257a57a9bd7a48142fb","recordedAt":"2026-08-20T09:30:36.156240738Z"}
```

- git status --short

```text
（干净）
```

### S1.12 APPEND 重放

**操作**

```bash
kc append --as kai --command-id append-evt-1 --repo kr://acme/personals/kai --stream practice --event-id evt-1 --payload '{"note":"退款口径疑问"}'
```

**观测结果**

- 退出码：`0`
- stdout：

```json
{
  "receiptRef": "receipt:append:append-evt-1",
  "commandId": "append-evt-1",
  "surface": "APPEND",
  "disposition": "REPLAYED",
  "result": {
    "repositoryId": "kr://acme/personals/kai",
    "streamRef": "practice",
    "cursor": "1",
    "appended": [
      "rec-1"
    ]
  }
}
```

符合预期：`REPLAYED`。

### S1.13 同 event-id 异 payload

**操作**

```bash
kc append --as kai --command-id append-evt-1-bad --repo kr://acme/personals/kai --stream practice --event-id evt-1 --payload '{"note":"different"}'
```

**观测结果**

- 退出码：`1`
- stdout：

```json
{
  "error": {
    "code": "EVENT_ID_CONFLICT",
    "message": "event id evt-1 already used with different payload"
  }
}
```

符合预期：`EVENT_ID_CONFLICT`。

### S1.14 流 lookup

**操作**

```bash
kc stream --repo kr://acme/personals/kai --stream practice --event-id evt-1
```

**观测结果**

- 退出码：`0`
- stdout：

```json
{
  "repository": "kr://acme/personals/kai",
  "streamRef": "practice",
  "face": "lookup",
  "headCursor": "1",
  "nextCursor": "1",
  "hasMore": false,
  "completeness": "durable",
  "cursor": "1",
  "records": [
    {
      "recordId": "rec-1",
      "eventId": "evt-1",
      "payload": {
        "note": "退款口径疑问"
      },
      "digest": "647aa423944464b165f58d47717814a3575b2e2d903ce257a57a9bd7a48142fb",
      "recordedAt": "2026-08-20T09:30:36.156240738Z"
    }
  ]
}
```

当时磁盘 / 其它命令：

- 登记表 git（写入知识后应不变）

```text
33059b5 register kr://acme/personals/kai
58725c2 register kr://acme/org/semantics
9ab2976 register kr://acme/public/metadata
ab52bed init kr://acme/catalog
396d4d1 root
```

- 登记表文件（仍无 view/release）

```text
catalog.yaml
repository-kr_acme_org_semantics.yaml
repository-kr_acme_personals_kai.yaml
repository-kr_acme_public_metadata.yaml
```

## S2　公司板第一代（还没有认领的 GMV）

### S2.1 define-view

**操作**

```bash
kc define-view --as steward --request-id s2-define --view analyst-board --revision 1 --source kr://acme/public/metadata=refs/heads/main --source kr://acme/org/semantics=refs/heads/main
```

**观测结果**

- 退出码：`0`
- stdout：

```json
{
  "viewId": "analyst-board",
  "revision": 1,
  "sources": [
    {
      "repository": "kr://acme/public/metadata",
      "selector": "refs/heads/main"
    },
    {
      "repository": "kr://acme/org/semantics",
      "selector": "refs/heads/main"
    }
  ]
}
```

当时磁盘 / 其它命令：

- 登记表文件

```text
catalog.yaml
repository-kr_acme_org_semantics.yaml
repository-kr_acme_personals_kai.yaml
repository-kr_acme_public_metadata.yaml
view-analyst-board.yaml
```

- view-analyst-board.yaml

```yaml
revision: 1
sources:
  - repository: kr://acme/public/metadata
    selector: refs/heads/main
  - repository: kr://acme/org/semantics
    selector: refs/heads/main
viewId: analyst-board
```

- 登记表 git

```text
4515657 define-view analyst-board
33059b5 register kr://acme/personals/kai
58725c2 register kr://acme/org/semantics
9ab2976 register kr://acme/public/metadata
ab52bed init kr://acme/catalog
396d4d1 root
```

### S2.2 define-view 之后仍无 Release

**操作**

```bash
kc read --release stable --object Table:dwd.trade_order
```

**观测结果**

- 退出码：`1`
- stdout：

```json
{
  "error": {
    "code": "VIEW_GENERATION_INVALID",
    "message": "unknown release stable"
  }
}
```

符合预期：配方还不是读者坐标。

### S2.3 promote --view（pin + CAS）

**操作**

```bash
kc promote --as steward --request-id s2-promote --release stable --view analyst-board
```

**观测结果**

- 退出码：`0`
- stdout：

```json
{
  "release": "stable",
  "generationId": "f5daa2aac586704da4a0c64335adf71f9340fc435cd131cf762f79f4fd30c715",
  "generation": {
    "generationId": "f5daa2aac586704da4a0c64335adf71f9340fc435cd131cf762f79f4fd30c715",
    "definitionRevision": 1,
    "repositories": {
      "kr://acme/org/semantics": "b82d56897213444f52f655c28261986a5ba02d13",
      "kr://acme/public/metadata": "b40f0ddf703ded326109f2fdf56ff265d0d9009b"
    }
  }
}
```

当时磁盘 / 其它命令：

- 登记表文件

```text
catalog.yaml
generation-f5daa2aac586704da4a0c64335adf71f9340fc435cd131cf762f79f4fd30c715.yaml
repository-kr_acme_org_semantics.yaml
repository-kr_acme_personals_kai.yaml
repository-kr_acme_public_metadata.yaml
release-stable.yaml
view-analyst-board.yaml
```

- release-stable.yaml

```yaml
generationId: f5daa2aac586704da4a0c64335adf71f9340fc435cd131cf762f79f4fd30c715
name: stable
```

- generation yaml

```yaml
definitionRevision: 1
generationId: f5daa2aac586704da4a0c64335adf71f9340fc435cd131cf762f79f4fd30c715
repositories:
  kr://acme/org/semantics: b82d56897213444f52f655c28261986a5ba02d13
  kr://acme/public/metadata: b40f0ddf703ded326109f2fdf56ff265d0d9009b
```

- 登记表 git

```text
dbeafbd promote stable -> f5daa2aac586
4ea47eb pin-view analyst-board
4515657 define-view analyst-board
33059b5 register kr://acme/personals/kai
58725c2 register kr://acme/org/semantics
9ab2976 register kr://acme/public/metadata
ab52bed init kr://acme/catalog
396d4d1 root
```

### S2.4 分析 Agent 读表

**操作**

```bash
kc read --as analyst-agent --release stable --object Table:dwd.trade_order
```

**观测结果**

- 退出码：`0`
- stdout：

```json
[
  {
    "repository": "kr://acme/public/metadata",
    "commit": "b40f0ddf703ded326109f2fdf56ff265d0d9009b",
    "objectId": "Table:dwd.trade_order",
    "value": {
      "ownership": {
        "owner": "platform"
      },
      "structure": {
        "db": "dw",
        "name": "dwd.trade_order"
      }
    }
  }
]
```

### S2.5 分析 Agent 读 Metric:gmv（应为空数组）

**操作**

```bash
kc read --as analyst-agent --release stable --object Metric:gmv
```

**观测结果**

- 退出码：`0`
- stdout：

```json
[]
```

符合预期：G1 上口径未认领。空数组 `[]` 不是错误。

### S2.6 resolve 例题

**操作**

```bash
kc resolve --as analyst-agent --release stable --object Example:gmv-refund
```

**观测结果**

- 退出码：`0`
- stdout：

```json
[
  {
    "repository": "kr://acme/org/semantics",
    "commit": "b82d56897213444f52f655c28261986a5ba02d13",
    "objectId": "Example:gmv-refund",
    "address": {
      "kind": "Entity",
      "objectId": "Example:gmv-refund"
    },
    "pathHint": "examples/gmv-refund.md",
    "digest": "97dac13c5ced266d9467d330e14b3dfb64ff8e77605ee72887900b639422790a",
    "schemaRef": "schema/example.body",
    "status": "RESOLVED"
  }
]
```

### S2.7 status

**操作**

```bash
kc status
```

**观测结果**

- 退出码：`0`
- stdout：

```json
{
  "archived": false,
  "catalog": {
    "head": "dbeafbd890413ea1b3d55403884cab212a84d034",
    "repositoryId": "kr://acme/catalog"
  },
  "catalogs": [
    {
      "dir": "catalogs/kr_acme_catalog",
      "head": "dbeafbd890413ea1b3d55403884cab212a84d034",
      "id": "kr://acme/catalog"
    }
  ],
  "generations": [
    {
      "generationId": "f5daa2aac586704da4a0c64335adf71f9340fc435cd131cf762f79f4fd30c715",
      "definitionRevision": 1,
      "repositories": {
        "kr://acme/org/semantics": "b82d56897213444f52f655c28261986a5ba02d13",
        "kr://acme/public/metadata": "b40f0ddf703ded326109f2fdf56ff265d0d9009b"
      }
    }
  ],
  "repositories": [
    "kr://acme/org/semantics",
    "kr://acme/personals/kai",
    "kr://acme/public/metadata"
  ],
  "namespace": "acme",
  "releases": {
    "stable": "f5daa2aac586704da4a0c64335adf71f9340fc435cd131cf762f79f4fd30c715"
  },
  "repos": [
    {
      "archived": false,
      "dir": "repos/kr_acme_org_semantics",
      "driver": "filegit",
      "head": "b82d56897213444f52f655c28261986a5ba02d13",
      "id": "kr://acme/org/semantics"
    },
    {
      "archived": false,
      "dir": "repos/kr_acme_personals_kai",
      "driver": "filegit",
      "head": "7b0056ae331eefbbf000695bda059c9e958bf322",
      "id": "kr://acme/personals/kai"
    },
    {
      "archived": false,
      "dir": "repos/kr_acme_public_metadata",
      "driver": "filegit",
      "head": "b40f0ddf703ded326109f2fdf56ff265d0d9009b",
      "id": "kr://acme/public/metadata"
    }
  ],
  "retiredReleases": {},
  "stores": {
    "index": "sqlite",
    "layout": {
      "catalogs": "catalogs",
      "projections": "projections",
      "repos": "repos"
    },
    "profile": "local",
    "repository": "filegit",
    "secrets": {
      "elasticsearch": "KC_ELASTICSEARCH_PASSWORD or KC_ELASTICSEARCH_API_KEY",
      "gitea": "KC_GITEA_TOKEN",
      "redis": "KC_REDIS_PASSWORD",
      "starrocks": "KC_STARROCKS_PASSWORD"
    }
  },
  "views": [
    {
      "viewId": "analyst-board",
      "revision": 1,
      "sources": [
        {
          "repository": "kr://acme/public/metadata",
          "selector": "refs/heads/main"
        },
        {
          "repository": "kr://acme/org/semantics",
          "selector": "refs/heads/main"
        }
      ]
    }
  ]
}
```

## S3　认领 GMV：propose ≠ merge ≠ promote

### S3.1 propose（只写候选 Ref）

**操作**

```bash
kc propose --as steward --proposal-id PR-gmv --repo kr://acme/org/semantics --target refs/heads/main --candidate refs/heads/candidates/PR-gmv --object Metric:gmv --aspect definition --schema-ref schema/metric.definition --path-hint metrics/gmv.json --value '{"formula":"GMV 不含 7 日内退货","description":"组织认领的交易额口径"}' --origin-kind DEFINITION --actor-ref steward --message '认领 GMV：不含 7 日内退货'
```

**观测结果**

- 退出码：`0`
- stdout：

```json
{
  "proposalId": "PR-gmv",
  "targetRepository": "kr://acme/org/semantics",
  "targetRef": "refs/heads/main",
  "candidateRef": "refs/heads/candidates/PR-gmv",
  "baseCommit": "b82d56897213444f52f655c28261986a5ba02d13",
  "candidateCommit": "40ab9f1e5d5a72c880c28136feb41a7129f41412",
  "rationale": "认领 GMV：不含 7 日内退货"
}
```

当时磁盘 / 其它命令：

- show-ref

```text
40ab9f1e5d5a72c880c28136feb41a7129f41412 refs/heads/candidates/PR-gmv
b82d56897213444f52f655c28261986a5ba02d13 refs/heads/main
```

- main `b82d56897213444f52f655c28261986a5ba02d13` vs S1 `b82d56897213444f52f655c28261986a5ba02d13`（应相等）
- 候选 tree

```text
100644 blob 2c1852b5af1912dc6c85c601b58d50a8b1aa82bc	examples/gmv-refund.md
100644 blob daf630b59a807ea9eafd7e851bbfbfdde9d45d5a	metrics/gmv.json
100644 blob b0e66908c6d56624f0738b82c4dd63b3a1762855	objects/schema/example.body.json
100644 blob a0dda6fca324259eb4f5f017cd35a8ab1e3d2b36	objects/schema/metric.definition.json
```

- 候选 metrics/gmv.json

```text
---
object_id: Metric:gmv
aspect_name: definition
kind: Aspect
path_hint: metrics/gmv.json
schema_ref: schema/metric.definition
provenance: {"originKind":"DEFINITION","actorRef":"steward"}
---
{
  "description": "组织认领的交易额口径",
  "formula": "GMV 不含 7 日内退货"
}
```

- main 上 `HEAD:metrics/gmv.json`：`git cat-file -e` 退出码 `128`（非 0 表示还没有）
- 登记表 git（propose 不应新增）

```text
dbeafbd promote stable -> f5daa2aac586
4ea47eb pin-view analyst-board
4515657 define-view analyst-board
33059b5 register kr://acme/personals/kai
58725c2 register kr://acme/org/semantics
9ab2976 register kr://acme/public/metadata
ab52bed init kr://acme/catalog
396d4d1 root
```

### S3.2 preview（登记 Gpv，不改 Release）

**操作**

```bash
kc preview --as steward --proposal PR-gmv --base-generation f5daa2aac586704da4a0c64335adf71f9340fc435cd131cf762f79f4fd30c715
```

**观测结果**

- 退出码：`0`
- stdout：

```json
{
  "previewId": "preview-90b27ad10a46b3b9be0357e5ca6dbba4678d9ce109fff4ed4e2e40a359eb4781",
  "baseGenerationId": "f5daa2aac586704da4a0c64335adf71f9340fc435cd131cf762f79f4fd30c715",
  "generation": {
    "generationId": "90b27ad10a46b3b9be0357e5ca6dbba4678d9ce109fff4ed4e2e40a359eb4781",
    "definitionRevision": 1,
    "repositories": {
      "kr://acme/org/semantics": "40ab9f1e5d5a72c880c28136feb41a7129f41412",
      "kr://acme/public/metadata": "b40f0ddf703ded326109f2fdf56ff265d0d9009b"
    }
  },
  "candidate": {
    "repositoryId": "kr://acme/org/semantics",
    "commitId": "40ab9f1e5d5a72c880c28136feb41a7129f41412"
  }
}
```

当时磁盘 / 其它命令：

- generation 文件

```text
generation-90b27ad10a46b3b9be0357e5ca6dbba4678d9ce109fff4ed4e2e40a359eb4781.yaml
generation-f5daa2aac586704da4a0c64335adf71f9340fc435cd131cf762f79f4fd30c715.yaml
```

- generation yaml

```yaml
definitionRevision: 1
generationId: 90b27ad10a46b3b9be0357e5ca6dbba4678d9ce109fff4ed4e2e40a359eb4781
repositories:
  kr://acme/org/semantics: 40ab9f1e5d5a72c880c28136feb41a7129f41412
  kr://acme/public/metadata: b40f0ddf703ded326109f2fdf56ff265d0d9009b

---
definitionRevision: 1
generationId: f5daa2aac586704da4a0c64335adf71f9340fc435cd131cf762f79f4fd30c715
repositories:
  kr://acme/org/semantics: b82d56897213444f52f655c28261986a5ba02d13
  kr://acme/public/metadata: b40f0ddf703ded326109f2fdf56ff265d0d9009b
```

- release-stable.yaml（应仍为 G1）

```yaml
generationId: f5daa2aac586704da4a0c64335adf71f9340fc435cd131cf762f79f4fd30c715
name: stable
```

### S3.3 在候选 commit 上读 GMV（维护口）

**操作**

```bash
kc read --repo kr://acme/org/semantics --object Metric:gmv --commit 40ab9f1e5d5a72c880c28136feb41a7129f41412
```

**观测结果**

- 退出码：`0`
- stdout：

```json
{
  "knowledgeRef": {
    "repository": "kr://acme/org/semantics",
    "object": "Metric:gmv"
  },
  "repository": "kr://acme/org/semantics",
  "commit": "40ab9f1e5d5a72c880c28136feb41a7129f41412",
  "address": {
    "kind": "Entity",
    "objectId": "Metric:gmv"
  },
  "value": {
    "definition": {
      "description": "组织认领的交易额口径",
      "formula": "GMV 不含 7 日内退货"
    }
  },
  "provenance": {
    "originKind": "DEFINITION",
    "actorRef": "steward"
  },
  "units": [
    {
      "kind": "Aspect",
      "objectId": "Metric:gmv",
      "aspectName": "definition"
    }
  ]
}
```

### S3.4 读者此时仍看不到 GMV

**操作**

```bash
kc read --as analyst-agent --release stable --object Metric:gmv
```

**观测结果**

- 退出码：`0`
- stdout：

```json
[]
```

符合预期：preview 不是 Release。

### S3.5 validate structure

**操作**

```bash
kc validate --as steward --preview preview-90b27ad10a46b3b9be0357e5ca6dbba4678d9ce109fff4ed4e2e40a359eb4781
```

**观测结果**

- 退出码：`0`
- stdout：

```json
{
  "reportId": "val-90b27ad10a46b3b9be0357e5ca6dbba4678d9ce109fff4ed4e2e40a359eb4781-structure",
  "previewGenerationId": "90b27ad10a46b3b9be0357e5ca6dbba4678d9ce109fff4ed4e2e40a359eb4781",
  "suiteRevision": "structure",
  "outcome": "PASSED",
  "check": {
    "generationId": "90b27ad10a46b3b9be0357e5ca6dbba4678d9ce109fff4ed4e2e40a359eb4781",
    "outcome": "PASSED",
    "issues": []
  }
}
```

### S3.6 merge 缺 steward 套件

**操作**

```bash
kc merge --as steward --repo kr://acme/org/semantics --proposal PR-gmv --preview preview-90b27ad10a46b3b9be0357e5ca6dbba4678d9ce109fff4ed4e2e40a359eb4781
```

**观测结果**

- 退出码：`1`
- stdout：

```json
{
  "error": {
    "code": "GATE_UNSATISFIED",
    "message": "gate suite:steward is not PASSED on this basis"
  }
}
```

当时磁盘 / 其它命令：

- merge 后 main：`b82d56897213444f52f655c28261986a5ba02d13`（应仍为 S1）
- release-stable.yaml

```yaml
generationId: f5daa2aac586704da4a0c64335adf71f9340fc435cd131cf762f79f4fd30c715
name: stable
```

符合预期：`GATE_UNSATISFIED`。若漏 `--repo`，观测会变成 `FORBIDDEN`（allow），不是 gate。

### S3.7 record-validation steward PASSED

**操作**

```bash
kc record-validation --as steward --preview preview-90b27ad10a46b3b9be0357e5ca6dbba4678d9ce109fff4ed4e2e40a359eb4781 --suite steward --outcome PASSED
```

**观测结果**

- 退出码：`0`
- stdout：

```json
{
  "reportId": "val-90b27ad10a46b3b9be0357e5ca6dbba4678d9ce109fff4ed4e2e40a359eb4781-steward",
  "previewGenerationId": "90b27ad10a46b3b9be0357e5ca6dbba4678d9ce109fff4ed4e2e40a359eb4781",
  "suiteRevision": "steward",
  "outcome": "PASSED"
}
```

### S3.8 merge（main 快进；Release 仍 G1）

**操作**

```bash
kc merge --as steward --repo kr://acme/org/semantics --proposal PR-gmv --preview preview-90b27ad10a46b3b9be0357e5ca6dbba4678d9ce109fff4ed4e2e40a359eb4781
```

**观测结果**

- 退出码：`0`
- stdout：

```json
{
  "commitId": "40ab9f1e5d5a72c880c28136feb41a7129f41412",
  "note": "target Ref moved; Release unchanged — run promote --view to serve"
}
```

当时磁盘 / 其它命令：

- main `40ab9f1e5d5a72c880c28136feb41a7129f41412`（应等于 Cpv `40ab9f1e5d5a72c880c28136feb41a7129f41412`）
- main tree（应有 metrics/gmv.json）

```text
100644 blob 2c1852b5af1912dc6c85c601b58d50a8b1aa82bc	examples/gmv-refund.md
100644 blob daf630b59a807ea9eafd7e851bbfbfdde9d45d5a	metrics/gmv.json
100644 blob b0e66908c6d56624f0738b82c4dd63b3a1762855	objects/schema/example.body.json
100644 blob a0dda6fca324259eb4f5f017cd35a8ab1e3d2b36	objects/schema/metric.definition.json
```

- release-stable.yaml（应仍 G1）

```yaml
generationId: f5daa2aac586704da4a0c64335adf71f9340fc435cd131cf762f79f4fd30c715
name: stable
```

### S3.9 merge 后读者仍跟 G1

**操作**

```bash
kc read --as analyst-agent --release stable --object Metric:gmv
```

**观测结果**

- 退出码：`0`
- stdout：

```json
[]
```

符合预期：merge ≠ promote，仍是 `[]`。

### S3.10 promote --expected G1

**操作**

```bash
kc promote --as steward --release stable --view analyst-board --expected f5daa2aac586704da4a0c64335adf71f9340fc435cd131cf762f79f4fd30c715
```

**观测结果**

- 退出码：`0`
- stdout：

```json
{
  "release": "stable",
  "generationId": "90b27ad10a46b3b9be0357e5ca6dbba4678d9ce109fff4ed4e2e40a359eb4781",
  "generation": {
    "generationId": "90b27ad10a46b3b9be0357e5ca6dbba4678d9ce109fff4ed4e2e40a359eb4781",
    "definitionRevision": 1,
    "repositories": {
      "kr://acme/org/semantics": "40ab9f1e5d5a72c880c28136feb41a7129f41412",
      "kr://acme/public/metadata": "b40f0ddf703ded326109f2fdf56ff265d0d9009b"
    }
  }
}
```

当时磁盘 / 其它命令：

- release-stable.yaml

```yaml
generationId: 90b27ad10a46b3b9be0357e5ca6dbba4678d9ce109fff4ed4e2e40a359eb4781
name: stable
```

- generation 文件

```text
generation-90b27ad10a46b3b9be0357e5ca6dbba4678d9ce109fff4ed4e2e40a359eb4781.yaml
generation-f5daa2aac586704da4a0c64335adf71f9340fc435cd131cf762f79f4fd30c715.yaml
```

- 登记表 git

```text
e445ac8 promote stable -> 90b27ad10a46
84cb52d create-preview 90b27ad10a46b3b9be0357e5ca6dbba4678d9ce109fff4ed4e2e40a359eb4781
dbeafbd promote stable -> f5daa2aac586
4ea47eb pin-view analyst-board
4515657 define-view analyst-board
33059b5 register kr://acme/personals/kai
58725c2 register kr://acme/org/semantics
9ab2976 register kr://acme/public/metadata
ab52bed init kr://acme/catalog
396d4d1 root
```

### S3.11 读者终于读到认领口径

**操作**

```bash
kc read --as analyst-agent --release stable --object Metric:gmv
```

**观测结果**

- 退出码：`0`
- stdout：

```json
[
  {
    "repository": "kr://acme/org/semantics",
    "commit": "40ab9f1e5d5a72c880c28136feb41a7129f41412",
    "objectId": "Metric:gmv",
    "value": {
      "definition": {
        "description": "组织认领的交易额口径",
        "formula": "GMV 不含 7 日内退货"
      }
    }
  }
]
```

当时磁盘 / 其它命令：

- G2 `90b27ad10a46b3b9be0357e5ca6dbba4678d9ce109fff4ed4e2e40a359eb4781` vs Gpv `90b27ad10a46b3b9be0357e5ca6dbba4678d9ce109fff4ed4e2e40a359eb4781`（快进合并时应相同）

## S4　个人桌面；草稿不进 stable；overlay 不覆盖

### S4.1 define-view kai-desk

**操作**

```bash
kc define-view --view kai-desk --revision 1 --source kr://acme/personals/kai=refs/heads/main
```

**观测结果**

- 退出码：`0`
- stdout：

```json
{
  "viewId": "kai-desk",
  "revision": 1,
  "sources": [
    {
      "repository": "kr://acme/personals/kai",
      "selector": "refs/heads/main"
    }
  ]
}
```

当时磁盘 / 其它命令：

- view-kai-desk.yaml

```yaml
revision: 1
sources:
  - repository: kr://acme/personals/kai
    selector: refs/heads/main
viewId: kai-desk
```

### S4.2 promote desk

**操作**

```bash
kc promote --release desk --view kai-desk
```

**观测结果**

- 退出码：`0`
- stdout：

```json
{
  "release": "desk",
  "generationId": "09956d653ae5304ffdac92d7d8c8ca56234be4ac2746d0baf0a91161000a0dbd",
  "generation": {
    "generationId": "09956d653ae5304ffdac92d7d8c8ca56234be4ac2746d0baf0a91161000a0dbd",
    "definitionRevision": 1,
    "repositories": {
      "kr://acme/personals/kai": "7b0056ae331eefbbf000695bda059c9e958bf322"
    }
  }
}
```

当时磁盘 / 其它命令：

- release-desk.yaml

```yaml
generationId: 09956d653ae5304ffdac92d7d8c8ca56234be4ac2746d0baf0a91161000a0dbd
name: desk
```

### S4.3 desk 读习惯（应钉 K1）

**操作**

```bash
kc read --as kai --release desk --object Habit:morning-review
```

**观测结果**

- 退出码：`0`
- stdout：

```json
[
  {
    "repository": "kr://acme/personals/kai",
    "commit": "7b0056ae331eefbbf000695bda059c9e958bf322",
    "objectId": "Habit:morning-review",
    "value": {
      "note": {
        "text": "每天先看昨日异常单",
        "when": "morning"
      }
    }
  }
]
```

### S4.4 desk 此时没有 Metric:gmv

**操作**

```bash
kc read --as kai --release desk --object Metric:gmv
```

**观测结果**

- 退出码：`0`
- stdout：

```json
[]
```

符合预期：K1 还没有草稿 GMV。

### S4.5 个人草稿 GMV（K2）

**操作**

```bash
kc put --as kai --command-id kai-gmv-draft --repo kr://acme/personals/kai --object Metric:gmv --aspect definition --path-hint drafts/gmv.json --value '{"formula":"GMV 含税，个人草稿","description":"未发表的个人理解"}' --origin-kind OBSERVATION --actor-ref kai
```

**观测结果**

- 退出码：`0`
- stdout：

```json
{
  "receiptRef": "receipt:commit:8dae467790626629939a73b0992fd7be4753e750",
  "commandId": "kai-gmv-draft",
  "surface": "COMMIT",
  "disposition": "APPLIED",
  "result": {
    "repositoryId": "kr://acme/personals/kai",
    "commitId": "8dae467790626629939a73b0992fd7be4753e750",
    "targetRef": "refs/heads/main",
    "oldCommit": "7b0056ae331eefbbf000695bda059c9e958bf322",
    "newCommit": "8dae467790626629939a73b0992fd7be4753e750"
  }
}
```

当时磁盘 / 其它命令：

- ls-tree HEAD

```text
100644 blob c14b1519460d680621cead7faccc9740f580f49c	drafts/gmv.json
100644 blob 72ebf2967a640007e8bf75ffb1295ccc72ab39b2	objects/Dist:error-by-topic/stats.json
100644 blob f2f7818e71d1ba380c73687df921f6af6df504a8	objects/Habit:morning-review/note.json
100644 blob cdafea631b9642d04499d4352ae5d488b8fce237	objects/schema/dist.stats.json
100644 blob 8eac79aa4ccacb42620a857d90751624e10cccec	objects/schema/habit.note.json
```

- drafts/gmv.json

```text
---
object_id: Metric:gmv
aspect_name: definition
kind: Aspect
path_hint: drafts/gmv.json
provenance: {"originKind":"OBSERVATION","actorRef":"kai"}
---
{
  "description": "未发表的个人理解",
  "formula": "GMV 含税，个人草稿"
}
```

- release-desk.yaml（仍应钉 K1）

```yaml
generationId: 09956d653ae5304ffdac92d7d8c8ca56234be4ac2746d0baf0a91161000a0dbd
name: desk
```

- release-stable.yaml（仍应 G2）

```yaml
generationId: 90b27ad10a46b3b9be0357e5ca6dbba4678d9ce109fff4ed4e2e40a359eb4781
name: stable
```

### S4.6 desk 仍无 GMV（不跟随 main）

**操作**

```bash
kc read --as kai --release desk --object Metric:gmv
```

**观测结果**

- 退出码：`0`
- stdout：

```json
[]
```

### S4.7 stable 仍是组织口径

**操作**

```bash
kc read --as analyst-agent --release stable --object Metric:gmv
```

**观测结果**

- 退出码：`0`
- stdout：

```json
[
  {
    "repository": "kr://acme/org/semantics",
    "commit": "40ab9f1e5d5a72c880c28136feb41a7129f41412",
    "objectId": "Metric:gmv",
    "value": {
      "definition": {
        "description": "组织认领的交易额口径",
        "formula": "GMV 不含 7 日内退货"
      }
    }
  }
]
```

### S4.8 反例：配方加入 personal（revision 2）

**操作**

```bash
kc define-view --as steward --view analyst-board --revision 2 --source kr://acme/public/metadata=refs/heads/main --source kr://acme/org/semantics=refs/heads/main --source kr://acme/personals/kai=refs/heads/main
```

**观测结果**

- 退出码：`0`
- stdout：

```json
{
  "viewId": "analyst-board",
  "revision": 2,
  "sources": [
    {
      "repository": "kr://acme/public/metadata",
      "selector": "refs/heads/main"
    },
    {
      "repository": "kr://acme/org/semantics",
      "selector": "refs/heads/main"
    },
    {
      "repository": "kr://acme/personals/kai",
      "selector": "refs/heads/main"
    }
  ]
}
```

当时磁盘 / 其它命令：

- view-analyst-board.yaml

```yaml
revision: 2
sources:
  - repository: kr://acme/public/metadata
    selector: refs/heads/main
  - repository: kr://acme/org/semantics
    selector: refs/heads/main
  - repository: kr://acme/personals/kai
    selector: refs/heads/main
viewId: analyst-board
```

### S4.9 pin overlay（不 promote stable）

**操作**

```bash
kc pin-view --as steward --view analyst-board
```

**观测结果**

- 退出码：`0`
- stdout：

```json
{
  "generationId": "dcdc2c8e204e8b567b418e586e60ae77bda82188eb284f91cf3c1dda8f267285",
  "definitionRevision": 2,
  "repositories": {
    "kr://acme/org/semantics": "40ab9f1e5d5a72c880c28136feb41a7129f41412",
    "kr://acme/personals/kai": "8dae467790626629939a73b0992fd7be4753e750",
    "kr://acme/public/metadata": "b40f0ddf703ded326109f2fdf56ff265d0d9009b"
  }
}
```

当时磁盘 / 其它命令：

- generation 文件

```text
generation-09956d653ae5304ffdac92d7d8c8ca56234be4ac2746d0baf0a91161000a0dbd.yaml
generation-90b27ad10a46b3b9be0357e5ca6dbba4678d9ce109fff4ed4e2e40a359eb4781.yaml
generation-dcdc2c8e204e8b567b418e586e60ae77bda82188eb284f91cf3c1dda8f267285.yaml
generation-f5daa2aac586704da4a0c64335adf71f9340fc435cd131cf762f79f4fd30c715.yaml
```

- 全部 generation yaml

```yaml
definitionRevision: 1
generationId: 09956d653ae5304ffdac92d7d8c8ca56234be4ac2746d0baf0a91161000a0dbd
repositories:
  kr://acme/personals/kai: 7b0056ae331eefbbf000695bda059c9e958bf322

---
definitionRevision: 1
generationId: 90b27ad10a46b3b9be0357e5ca6dbba4678d9ce109fff4ed4e2e40a359eb4781
repositories:
  kr://acme/org/semantics: 40ab9f1e5d5a72c880c28136feb41a7129f41412
  kr://acme/public/metadata: b40f0ddf703ded326109f2fdf56ff265d0d9009b

---
definitionRevision: 2
generationId: dcdc2c8e204e8b567b418e586e60ae77bda82188eb284f91cf3c1dda8f267285
repositories:
  kr://acme/org/semantics: 40ab9f1e5d5a72c880c28136feb41a7129f41412
  kr://acme/personals/kai: 8dae467790626629939a73b0992fd7be4753e750
  kr://acme/public/metadata: b40f0ddf703ded326109f2fdf56ff265d0d9009b

---
definitionRevision: 1
generationId: f5daa2aac586704da4a0c64335adf71f9340fc435cd131cf762f79f4fd30c715
repositories:
  kr://acme/org/semantics: b82d56897213444f52f655c28261986a5ba02d13
  kr://acme/public/metadata: b40f0ddf703ded326109f2fdf56ff265d0d9009b
```

- release-stable.yaml（不得变成 overlay）

```yaml
generationId: 90b27ad10a46b3b9be0357e5ca6dbba4678d9ce109fff4ed4e2e40a359eb4781
name: stable
```

### S4.10 overlay 上的组织 GMV（维护口 @ C1）

**操作**

```bash
kc read --repo kr://acme/org/semantics --object Metric:gmv --commit 40ab9f1e5d5a72c880c28136feb41a7129f41412
```

**观测结果**

- 退出码：`0`
- stdout：

```json
{
  "knowledgeRef": {
    "repository": "kr://acme/org/semantics",
    "object": "Metric:gmv"
  },
  "repository": "kr://acme/org/semantics",
  "commit": "40ab9f1e5d5a72c880c28136feb41a7129f41412",
  "address": {
    "kind": "Entity",
    "objectId": "Metric:gmv"
  },
  "value": {
    "definition": {
      "description": "组织认领的交易额口径",
      "formula": "GMV 不含 7 日内退货"
    }
  },
  "provenance": {
    "originKind": "DEFINITION",
    "actorRef": "steward"
  },
  "units": [
    {
      "kind": "Aspect",
      "objectId": "Metric:gmv",
      "aspectName": "definition"
    }
  ]
}
```

### S4.11 overlay 上的个人 GMV（维护口 @ K2）

**操作**

```bash
kc read --repo kr://acme/personals/kai --object Metric:gmv --commit 8dae467790626629939a73b0992fd7be4753e750
```

**观测结果**

- 退出码：`0`
- stdout：

```json
{
  "knowledgeRef": {
    "repository": "kr://acme/personals/kai",
    "object": "Metric:gmv"
  },
  "repository": "kr://acme/personals/kai",
  "commit": "8dae467790626629939a73b0992fd7be4753e750",
  "address": {
    "kind": "Entity",
    "objectId": "Metric:gmv"
  },
  "value": {
    "definition": {
      "description": "未发表的个人理解",
      "formula": "GMV 含税，个人草稿"
    }
  },
  "provenance": {
    "originKind": "OBSERVATION",
    "actorRef": "kai"
  },
  "units": [
    {
      "kind": "Aspect",
      "objectId": "Metric:gmv",
      "aspectName": "definition"
    }
  ]
}
```

符合预期：同一 object_id 两份值，公式不同，没有互相覆盖。

### S4.12 配方改回公司两仓（revision 3）

**操作**

```bash
kc define-view --as steward --view analyst-board --revision 3 --source kr://acme/public/metadata=refs/heads/main --source kr://acme/org/semantics=refs/heads/main
```

**观测结果**

- 退出码：`0`
- stdout：

```json
{
  "viewId": "analyst-board",
  "revision": 3,
  "sources": [
    {
      "repository": "kr://acme/public/metadata",
      "selector": "refs/heads/main"
    },
    {
      "repository": "kr://acme/org/semantics",
      "selector": "refs/heads/main"
    }
  ]
}
```

当时磁盘 / 其它命令：

- view-analyst-board.yaml

```yaml
revision: 3
sources:
  - repository: kr://acme/public/metadata
    selector: refs/heads/main
  - repository: kr://acme/org/semantics
    selector: refs/heads/main
viewId: analyst-board
```

- stable 仍 G2

```yaml
generationId: 90b27ad10a46b3b9be0357e5ca6dbba4678d9ce109fff4ed4e2e40a359eb4781
name: stable
```

## S5　维护 main，不 promote

### S5.1 path-hint 移动例题

**操作**

```bash
kc put --command-id sem-path-example --repo kr://acme/org/semantics --object Example:gmv-refund --aspect body --schema-ref schema/example.body --path-hint semantics/examples/gmv-refund.md --value '{"prompt":"退货是否算进 GMV？"}' --origin-kind DEFINITION --actor-ref steward
```

**观测结果**

- 退出码：`0`
- stdout：

```json
{
  "receiptRef": "receipt:commit:727c859dd036edd76f39fdac428cade2de533f43",
  "commandId": "sem-path-example",
  "surface": "COMMIT",
  "disposition": "APPLIED",
  "result": {
    "repositoryId": "kr://acme/org/semantics",
    "commitId": "727c859dd036edd76f39fdac428cade2de533f43",
    "targetRef": "refs/heads/main",
    "oldCommit": "40ab9f1e5d5a72c880c28136feb41a7129f41412",
    "newCommit": "727c859dd036edd76f39fdac428cade2de533f43"
  }
}
```

当时磁盘 / 其它命令：

- C1 tree（例题旧路径）

```text
100644 blob 2c1852b5af1912dc6c85c601b58d50a8b1aa82bc	examples/gmv-refund.md
100644 blob daf630b59a807ea9eafd7e851bbfbfdde9d45d5a	metrics/gmv.json
100644 blob b0e66908c6d56624f0738b82c4dd63b3a1762855	objects/schema/example.body.json
100644 blob a0dda6fca324259eb4f5f017cd35a8ab1e3d2b36	objects/schema/metric.definition.json
```

- main tree（例题新路径）

```text
100644 blob daf630b59a807ea9eafd7e851bbfbfdde9d45d5a	metrics/gmv.json
100644 blob b0e66908c6d56624f0738b82c4dd63b3a1762855	objects/schema/example.body.json
100644 blob a0dda6fca324259eb4f5f017cd35a8ab1e3d2b36	objects/schema/metric.definition.json
100644 blob 2dd22fbaefcb7bf480e8a6281fd90750602b0375	semantics/examples/gmv-refund.md
```

- 新路径文件正文

```text
---
object_id: Example:gmv-refund
aspect_name: body
kind: Aspect
path_hint: semantics/examples/gmv-refund.md
schema_ref: schema/example.body
provenance: {"originKind":"DEFINITION","actorRef":"steward"}
---
{
  "prompt": "退货是否算进 GMV？"
}
```

### S5.2 resolve 在 pin

**操作**

```bash
kc resolve --repo kr://acme/org/semantics --object Example:gmv-refund --commit 40ab9f1e5d5a72c880c28136feb41a7129f41412
```

**观测结果**

- 退出码：`0`
- stdout：

```json
{
  "repository": "kr://acme/org/semantics",
  "commit": "40ab9f1e5d5a72c880c28136feb41a7129f41412",
  "objectId": "Example:gmv-refund",
  "address": {
    "kind": "Entity",
    "objectId": "Example:gmv-refund"
  },
  "pathHint": "examples/gmv-refund.md",
  "digest": "97dac13c5ced266d9467d330e14b3dfb64ff8e77605ee72887900b639422790a",
  "schemaRef": "schema/example.body",
  "status": "RESOLVED"
}
```

### S5.3 resolve 在 main

**操作**

```bash
kc resolve --repo kr://acme/org/semantics --object Example:gmv-refund --ref refs/heads/main
```

**观测结果**

- 退出码：`0`
- stdout：

```json
{
  "repository": "kr://acme/org/semantics",
  "commit": "727c859dd036edd76f39fdac428cade2de533f43",
  "objectId": "Example:gmv-refund",
  "address": {
    "kind": "Entity",
    "objectId": "Example:gmv-refund"
  },
  "pathHint": "semantics/examples/gmv-refund.md",
  "digest": "97dac13c5ced266d9467d330e14b3dfb64ff8e77605ee72887900b639422790a",
  "schemaRef": "schema/example.body",
  "status": "RESOLVED"
}
```

### S5.4 只改 ownership

**操作**

```bash
kc put --as collector --command-id meta-owner --repo kr://acme/public/metadata --object Table:dwd.trade_order --aspect ownership --schema-ref schema/table.ownership --path-hint tables/dwd.trade_order.ownership.json --value '{"owner":"platform-ops"}' --origin-kind SOURCE --source-ref metastore --actor-ref collector
```

**观测结果**

- 退出码：`0`
- stdout：

```json
{
  "receiptRef": "receipt:commit:ba9d0cc8a6644b5f795b34a7fa2875f2a47a4761",
  "commandId": "meta-owner",
  "surface": "COMMIT",
  "disposition": "APPLIED",
  "result": {
    "repositoryId": "kr://acme/public/metadata",
    "commitId": "ba9d0cc8a6644b5f795b34a7fa2875f2a47a4761",
    "targetRef": "refs/heads/main",
    "oldCommit": "b40f0ddf703ded326109f2fdf56ff265d0d9009b",
    "newCommit": "ba9d0cc8a6644b5f795b34a7fa2875f2a47a4761"
  }
}
```

当时磁盘 / 其它命令：

- U1 文件

```text
---
object_id: Table:dwd.trade_order
aspect_name: ownership
kind: Aspect
path_hint: tables/dwd.trade_order.ownership.json
schema_ref: schema/table.ownership
provenance: {"originKind":"SOURCE","actorRef":"collector","sourceRefs":["metastore"]}
---
{
  "owner": "platform"
}
```

- HEAD 文件

```text
---
object_id: Table:dwd.trade_order
aspect_name: ownership
kind: Aspect
path_hint: tables/dwd.trade_order.ownership.json
schema_ref: schema/table.ownership
provenance: {"originKind":"SOURCE","actorRef":"collector","sourceRefs":["metastore"]}
---
{
  "owner": "platform-ops"
}
```

### S5.5 stable 仍读 U1 的 owner=platform

**操作**

```bash
kc read --as analyst-agent --release stable --object Table:dwd.trade_order
```

**观测结果**

- 退出码：`0`
- stdout：

```json
[
  {
    "repository": "kr://acme/public/metadata",
    "commit": "b40f0ddf703ded326109f2fdf56ff265d0d9009b",
    "objectId": "Table:dwd.trade_order",
    "value": {
      "ownership": {
        "owner": "platform"
      },
      "structure": {
        "db": "dw",
        "name": "dwd.trade_order"
      }
    }
  }
]
```

### S5.6 IfAbsent 打在已有 structure 上

**操作**

```bash
kc put --as collector --command-id meta-if-absent --repo kr://acme/public/metadata --object Table:dwd.trade_order --aspect structure --schema-ref schema/table.structure --if-absent --value '{"db":"dw","name":"dwd.trade_order"}'
```

**观测结果**

- 退出码：`1`
- stdout：

```json
{
  "error": {
    "code": "PRECONDITION_FAILED",
    "message": "Table:dwd.trade_order\u001fstructure\u001f already exists"
  }
}
```

当时磁盘 / 其它命令：

- HEAD 仍是 `ba9d0cc8a6644b5f795b34a7fa2875f2a47a4761`（IfAbsent 前 `ba9d0cc8a6644b5f795b34a7fa2875f2a47a4761`）

符合预期：`PRECONDITION_FAILED`，HEAD 不变。

### S5.7 过期 expectedTargetCommit

**操作**

```bash
kc put --as collector --command-id stale-cas --repo kr://acme/public/metadata --object Table:dwd.trade_order --aspect ownership --expected b40f0ddf703ded326109f2fdf56ff265d0d9009b --schema-ref schema/table.ownership --value '{"owner":"stale"}'
```

**观测结果**

- 退出码：`1`
- stdout：

```json
{
  "error": {
    "code": "NON_FAST_FORWARD",
    "message": "expected b40f0ddf703ded326109f2fdf56ff265d0d9009b but ref is ba9d0cc8a6644b5f795b34a7fa2875f2a47a4761"
  }
}
```

符合预期：`NON_FAST_FORWARD`。

### S5.8 provenance（信封，不是 git log）

**操作**

```bash
kc provenance --repo kr://acme/org/semantics --object Metric:gmv --commit 40ab9f1e5d5a72c880c28136feb41a7129f41412
```

**观测结果**

- 退出码：`0`
- stdout：

```json
{
  "repository": "kr://acme/org/semantics",
  "commit": "40ab9f1e5d5a72c880c28136feb41a7129f41412",
  "objectId": "Metric:gmv",
  "chain": [
    {
      "originKind": "DEFINITION",
      "actorRef": "steward"
    }
  ]
}
```

### S5.9 log 例题（path move 不改 digest 时仍落在引入 commit）

**操作**

```bash
kc log --repo kr://acme/org/semantics --object Example:gmv-refund --ref refs/heads/main
```

**观测结果**

- 退出码：`0`
- stdout：

```json
[
  {
    "commit": "b82d56897213444f52f655c28261986a5ba02d13",
    "status": "RESOLVED",
    "digest": "97dac13c5ced266d9467d330e14b3dfb64ff8e77605ee72887900b639422790a"
  },
  {
    "commit": "b82d56897213444f52f655c28261986a5ba02d13",
    "status": "RESOLVED",
    "digest": "97dac13c5ced266d9467d330e14b3dfb64ff8e77605ee72887900b639422790a"
  }
]
```

### S5.10 diff 表 U1..U2

**操作**

```bash
kc diff --repo kr://acme/public/metadata --object Table:dwd.trade_order --from b40f0ddf703ded326109f2fdf56ff265d0d9009b --to ba9d0cc8a6644b5f795b34a7fa2875f2a47a4761
```

**观测结果**

- 退出码：`0`
- stdout：

```json
{
  "objectId": "Table:dwd.trade_order",
  "fromCommit": "b40f0ddf703ded326109f2fdf56ff265d0d9009b",
  "toCommit": "ba9d0cc8a6644b5f795b34a7fa2875f2a47a4761",
  "from": {
    "knowledgeRef": {
      "repository": "kr://acme/public/metadata",
      "object": "Table:dwd.trade_order"
    },
    "repository": "kr://acme/public/metadata",
    "commit": "b40f0ddf703ded326109f2fdf56ff265d0d9009b",
    "address": {
      "kind": "Entity",
      "objectId": "Table:dwd.trade_order"
    },
    "value": {
      "ownership": {
        "owner": "platform"
      },
      "structure": {
        "db": "dw",
        "name": "dwd.trade_order"
      }
    },
    "units": [
      {
        "kind": "Aspect",
        "objectId": "Table:dwd.trade_order",
        "aspectName": "ownership"
      },
      {
        "kind": "Aspect",
        "objectId": "Table:dwd.trade_order",
        "aspectName": "structure"
      }
    ]
  },
  "to": {
    "knowledgeRef": {
      "repository": "kr://acme/public/metadata",
      "object": "Table:dwd.trade_order"
    },
    "repository": "kr://acme/public/metadata",
    "commit": "ba9d0cc8a6644b5f795b34a7fa2875f2a47a4761",
    "address": {
      "kind": "Entity",
      "objectId": "Table:dwd.trade_order"
    },
    "value": {
      "ownership": {
        "owner": "platform-ops"
      },
      "structure": {
        "db": "dw",
        "name": "dwd.trade_order"
      }
    },
    "units": [
      {
        "kind": "Aspect",
        "objectId": "Table:dwd.trade_order",
        "aspectName": "ownership"
      },
      {
        "kind": "Aspect",
        "objectId": "Table:dwd.trade_order",
        "aspectName": "structure"
      }
    ]
  }
}
```

### S5.11 index-plan --release stable

**操作**

```bash
kc index-plan --as steward --release stable
```

**观测结果**

- 退出码：`0`
- stdout：

```json
{
  "generationId": "90b27ad10a46b3b9be0357e5ca6dbba4678d9ce109fff4ed4e2e40a359eb4781",
  "definitionRevision": 1,
  "projections": [
    {
      "repository": "kr://acme/org/semantics",
      "commit": "40ab9f1e5d5a72c880c28136feb41a7129f41412",
      "schemaDigest": "67ad05e4f8a6da72af52dea9084750b800b05538d3a0805811e3a99b61faaf12",
      "schemas": [
        "schema/example.body",
        "schema/metric.definition"
      ],
      "fields": [
        {
          "schema": "schema/example.body",
          "entity": "Example",
          "aspect": "body",
          "path": "prompt",
          "access": [
            "text"
          ]
        },
        {
          "schema": "schema/metric.definition",
          "entity": "Metric",
          "aspect": "definition",
          "path": "description",
          "access": [
            "text",
            "summary"
          ]
        },
        {
          "schema": "schema/metric.definition",
          "entity": "Metric",
          "aspect": "definition",
          "path": "formula",
          "access": [
            "text"
          ]
        }
      ],
      "lanes": [
        "text"
      ]
    },
    {
      "repository": "kr://acme/public/metadata",
      "commit": "b40f0ddf703ded326109f2fdf56ff265d0d9009b",
      "schemaDigest": "9e26a39335412e72299c7d14c2240af7f283b5a84c1df8df33d5b99940199b36",
      "schemas": [
        "schema/table.ownership",
        "schema/table.structure"
      ],
      "fields": [
        {
          "schema": "schema/table.ownership",
          "entity": "Table",
          "aspect": "ownership",
          "path": "owner",
          "access": [
            "filter"
          ]
        },
        {
          "schema": "schema/table.structure",
          "entity": "Table",
          "aspect": "structure",
          "path": "db",
          "access": [
            "key",
            "filter"
          ]
        },
        {
          "schema": "schema/table.structure",
          "entity": "Table",
          "aspect": "structure",
          "path": "name",
          "access": [
            "filter",
            "text"
          ]
        }
      ],
      "lanes": [
        "key",
        "filter",
        "text"
      ]
    }
  ]
}
```

当时磁盘 / 其它命令：

- release-stable.yaml（维护不得移动它）

```yaml
generationId: 90b27ad10a46b3b9be0357e5ca6dbba4678d9ce109fff4ed4e2e40a359eb4781
name: stable
```

## S6　收场

### S6.1 retire-view

**操作**

```bash
kc retire-view --as steward --view analyst-board
```

**观测结果**

- 退出码：`0`
- stdout：

```json
{
  "retired": true,
  "view": "analyst-board"
}
```

当时磁盘 / 其它命令：

- view-analyst-board.yaml

```yaml
retired: true
revision: 3
sources:
  - repository: kr://acme/public/metadata
    selector: refs/heads/main
  - repository: kr://acme/org/semantics
    selector: refs/heads/main
viewId: analyst-board
```

### S6.2 配方退休后仍可读 stable

**操作**

```bash
kc read --as analyst-agent --release stable --object Metric:gmv
```

**观测结果**

- 退出码：`0`
- stdout：

```json
[
  {
    "repository": "kr://acme/org/semantics",
    "commit": "40ab9f1e5d5a72c880c28136feb41a7129f41412",
    "objectId": "Metric:gmv",
    "value": {
      "definition": {
        "description": "组织认领的交易额口径",
        "formula": "GMV 不含 7 日内退货"
      }
    }
  }
]
```

符合预期：读者跟 Generation，不是 ViewDefinition。

### S6.3 不能再 pin 已退休配方

**操作**

```bash
kc pin-view --as steward --view analyst-board
```

**观测结果**

- 退出码：`1`
- stdout：

```json
{
  "error": {
    "code": "VIEW_GENERATION_INVALID",
    "message": "view analyst-board is retired"
  }
}
```

符合预期：`VIEW_GENERATION_INVALID`。

### S6.4 retire-release stable

**操作**

```bash
kc retire-release --as steward --release stable
```

**观测结果**

- 退出码：`0`
- stdout：

```json
{
  "release": "stable",
  "retired": true
}
```

当时磁盘 / 其它命令：

- release-stable.yaml

```yaml
generationId: 90b27ad10a46b3b9be0357e5ca6dbba4678d9ce109fff4ed4e2e40a359eb4781
name: stable
retired: true
```

- G2 yaml 仍在

```yaml
definitionRevision: 1
generationId: 90b27ad10a46b3b9be0357e5ca6dbba4678d9ce109fff4ed4e2e40a359eb4781
repositories:
  kr://acme/org/semantics: 40ab9f1e5d5a72c880c28136feb41a7129f41412
  kr://acme/public/metadata: b40f0ddf703ded326109f2fdf56ff265d0d9009b
```

### S6.5 发布名停服

**操作**

```bash
kc read --as analyst-agent --release stable --object Metric:gmv
```

**观测结果**

- 退出码：`1`
- stdout：

```json
{
  "error": {
    "code": "VIEW_GENERATION_INVALID",
    "message": "unknown release stable"
  }
}
```

符合预期：unknown release。Generation 文件还在。

### S6.6 status（retiredReleases）

**操作**

```bash
kc status
```

**观测结果**

- 退出码：`0`
- stdout：

```json
{
  "archived": false,
  "catalog": {
    "head": "bc0d9211430fd3476e965c9c2f96c2a4569f937c",
    "repositoryId": "kr://acme/catalog"
  },
  "catalogs": [
    {
      "dir": "catalogs/kr_acme_catalog",
      "head": "bc0d9211430fd3476e965c9c2f96c2a4569f937c",
      "id": "kr://acme/catalog"
    }
  ],
  "generations": [
    {
      "generationId": "09956d653ae5304ffdac92d7d8c8ca56234be4ac2746d0baf0a91161000a0dbd",
      "definitionRevision": 1,
      "repositories": {
        "kr://acme/personals/kai": "7b0056ae331eefbbf000695bda059c9e958bf322"
      }
    },
    {
      "generationId": "90b27ad10a46b3b9be0357e5ca6dbba4678d9ce109fff4ed4e2e40a359eb4781",
      "definitionRevision": 1,
      "repositories": {
        "kr://acme/org/semantics": "40ab9f1e5d5a72c880c28136feb41a7129f41412",
        "kr://acme/public/metadata": "b40f0ddf703ded326109f2fdf56ff265d0d9009b"
      }
    },
    {
      "generationId": "dcdc2c8e204e8b567b418e586e60ae77bda82188eb284f91cf3c1dda8f267285",
      "definitionRevision": 2,
      "repositories": {
        "kr://acme/org/semantics": "40ab9f1e5d5a72c880c28136feb41a7129f41412",
        "kr://acme/personals/kai": "8dae467790626629939a73b0992fd7be4753e750",
        "kr://acme/public/metadata": "b40f0ddf703ded326109f2fdf56ff265d0d9009b"
      }
    },
    {
      "generationId": "f5daa2aac586704da4a0c64335adf71f9340fc435cd131cf762f79f4fd30c715",
      "definitionRevision": 1,
      "repositories": {
        "kr://acme/org/semantics": "b82d56897213444f52f655c28261986a5ba02d13",
        "kr://acme/public/metadata": "b40f0ddf703ded326109f2fdf56ff265d0d9009b"
      }
    }
  ],
  "repositories": [
    "kr://acme/public/metadata",
    "kr://acme/org/semantics",
    "kr://acme/personals/kai"
  ],
  "namespace": "acme",
  "releases": {
    "desk": "09956d653ae5304ffdac92d7d8c8ca56234be4ac2746d0baf0a91161000a0dbd"
  },
  "repos": [
    {
      "archived": false,
      "dir": "repos/kr_acme_org_semantics",
      "driver": "filegit",
      "head": "727c859dd036edd76f39fdac428cade2de533f43",
      "id": "kr://acme/org/semantics"
    },
    {
      "archived": false,
      "dir": "repos/kr_acme_personals_kai",
      "driver": "filegit",
      "head": "8dae467790626629939a73b0992fd7be4753e750",
      "id": "kr://acme/personals/kai"
    },
    {
      "archived": false,
      "dir": "repos/kr_acme_public_metadata",
      "driver": "filegit",
      "head": "ba9d0cc8a6644b5f795b34a7fa2875f2a47a4761",
      "id": "kr://acme/public/metadata"
    }
  ],
  "retiredReleases": {
    "stable": "90b27ad10a46b3b9be0357e5ca6dbba4678d9ce109fff4ed4e2e40a359eb4781"
  },
  "stores": {
    "index": "sqlite",
    "layout": {
      "catalogs": "catalogs",
      "projections": "projections",
      "repos": "repos"
    },
    "profile": "local",
    "repository": "filegit",
    "secrets": {
      "elasticsearch": "KC_ELASTICSEARCH_PASSWORD or KC_ELASTICSEARCH_API_KEY",
      "gitea": "KC_GITEA_TOKEN",
      "redis": "KC_REDIS_PASSWORD",
      "starrocks": "KC_STARROCKS_PASSWORD"
    }
  },
  "views": [
    {
      "viewId": "analyst-board",
      "revision": 3,
      "sources": [
        {
          "repository": "kr://acme/public/metadata",
          "selector": "refs/heads/main"
        },
        {
          "repository": "kr://acme/org/semantics",
          "selector": "refs/heads/main"
        }
      ],
      "retired": true
    },
    {
      "viewId": "kai-desk",
      "revision": 1,
      "sources": [
        {
          "repository": "kr://acme/personals/kai",
          "selector": "refs/heads/main"
        }
      ]
    }
  ]
}
```

### S6.7 archive-repo metadata

**操作**

```bash
kc archive-repo --repo kr://acme/public/metadata
```

**观测结果**

- 退出码：`0`
- stdout：

```json
{
  "archived": true,
  "repositoryId": "kr://acme/public/metadata"
}
```

当时磁盘 / 其它命令：

- show-ref（应有 refs/kc/archived）

```text
ba9d0cc8a6644b5f795b34a7fa2875f2a47a4761 refs/heads/main
ba9d0cc8a6644b5f795b34a7fa2875f2a47a4761 refs/kc/archived
```

### S6.8 归档后采集不能写

**操作**

```bash
kc put --as collector --command-id after-archive --repo kr://acme/public/metadata --object Table:dwd.trade_order --aspect ownership --schema-ref schema/table.ownership --value '{"owner":"gone"}'
```

**观测结果**

- 退出码：`1`
- stdout：

```json
{
  "error": {
    "code": "REPOSITORY_ARCHIVED",
    "message": "repository kr://acme/public/metadata is archived"
  }
}
```

符合预期：`REPOSITORY_ARCHIVED`。

### S6.9 archive-catalog

**操作**

```bash
kc archive-catalog --as steward --catalog kr://acme/catalog
```

**观测结果**

- 退出码：`0`
- stdout：

```json
{
  "archived": true,
  "catalog": "kr://acme/catalog"
}
```

当时磁盘 / 其它命令：

- catalog.yaml

```yaml
archived: true
id: kr://acme/catalog
```

### S6.10 归档后不能 define-view

**操作**

```bash
kc define-view --as steward --view late --revision 1 --source kr://acme/personals/kai=refs/heads/main
```

**观测结果**

- 退出码：`1`
- stdout：

```json
{
  "error": {
    "code": "CATALOG_ARCHIVED",
    "message": "catalog kr://acme/catalog is archived"
  }
}
```

符合预期：`CATALOG_ARCHIVED`。

### S6.11 个人仓仍可写

**操作**

```bash
kc put --as kai --command-id kai-after-archive --repo kr://acme/personals/kai --object Habit:morning-review --aspect note --schema-ref schema/habit.note --value '{"when":"morning","text":"catalog 归档不影响个人仓"}' --origin-kind OBSERVATION --actor-ref kai
```

**观测结果**

- 退出码：`0`
- stdout：

```json
{
  "receiptRef": "receipt:commit:b081b9ba1ce7e5d6f974b3845cd7ac38686fd91f",
  "commandId": "kai-after-archive",
  "surface": "COMMIT",
  "disposition": "APPLIED",
  "result": {
    "repositoryId": "kr://acme/personals/kai",
    "commitId": "b081b9ba1ce7e5d6f974b3845cd7ac38686fd91f",
    "targetRef": "refs/heads/main",
    "oldCommit": "8dae467790626629939a73b0992fd7be4753e750",
    "newCommit": "b081b9ba1ce7e5d6f974b3845cd7ac38686fd91f"
  }
}
```

### S6.12 最终 status

**操作**

```bash
kc status
```

**观测结果**

- 退出码：`0`
- stdout：

```json
{
  "archived": true,
  "catalog": {
    "head": "969c02ba1df8a153142e1578090ca4a40f3abc54",
    "repositoryId": "kr://acme/catalog"
  },
  "catalogs": [
    {
      "dir": "catalogs/kr_acme_catalog",
      "head": "969c02ba1df8a153142e1578090ca4a40f3abc54",
      "id": "kr://acme/catalog"
    }
  ],
  "generations": [
    {
      "generationId": "f5daa2aac586704da4a0c64335adf71f9340fc435cd131cf762f79f4fd30c715",
      "definitionRevision": 1,
      "repositories": {
        "kr://acme/org/semantics": "b82d56897213444f52f655c28261986a5ba02d13",
        "kr://acme/public/metadata": "b40f0ddf703ded326109f2fdf56ff265d0d9009b"
      }
    },
    {
      "generationId": "09956d653ae5304ffdac92d7d8c8ca56234be4ac2746d0baf0a91161000a0dbd",
      "definitionRevision": 1,
      "repositories": {
        "kr://acme/personals/kai": "7b0056ae331eefbbf000695bda059c9e958bf322"
      }
    },
    {
      "generationId": "90b27ad10a46b3b9be0357e5ca6dbba4678d9ce109fff4ed4e2e40a359eb4781",
      "definitionRevision": 1,
      "repositories": {
        "kr://acme/org/semantics": "40ab9f1e5d5a72c880c28136feb41a7129f41412",
        "kr://acme/public/metadata": "b40f0ddf703ded326109f2fdf56ff265d0d9009b"
      }
    },
    {
      "generationId": "dcdc2c8e204e8b567b418e586e60ae77bda82188eb284f91cf3c1dda8f267285",
      "definitionRevision": 2,
      "repositories": {
        "kr://acme/org/semantics": "40ab9f1e5d5a72c880c28136feb41a7129f41412",
        "kr://acme/personals/kai": "8dae467790626629939a73b0992fd7be4753e750",
        "kr://acme/public/metadata": "b40f0ddf703ded326109f2fdf56ff265d0d9009b"
      }
    }
  ],
  "repositories": [
    "kr://acme/public/metadata",
    "kr://acme/org/semantics",
    "kr://acme/personals/kai"
  ],
  "namespace": "acme",
  "releases": {
    "desk": "09956d653ae5304ffdac92d7d8c8ca56234be4ac2746d0baf0a91161000a0dbd"
  },
  "repos": [
    {
      "archived": false,
      "dir": "repos/kr_acme_org_semantics",
      "driver": "filegit",
      "head": "727c859dd036edd76f39fdac428cade2de533f43",
      "id": "kr://acme/org/semantics"
    },
    {
      "archived": false,
      "dir": "repos/kr_acme_personals_kai",
      "driver": "filegit",
      "head": "b081b9ba1ce7e5d6f974b3845cd7ac38686fd91f",
      "id": "kr://acme/personals/kai"
    },
    {
      "archived": true,
      "dir": "repos/kr_acme_public_metadata",
      "driver": "filegit",
      "head": "ba9d0cc8a6644b5f795b34a7fa2875f2a47a4761",
      "id": "kr://acme/public/metadata"
    }
  ],
  "retiredReleases": {
    "stable": "90b27ad10a46b3b9be0357e5ca6dbba4678d9ce109fff4ed4e2e40a359eb4781"
  },
  "stores": {
    "index": "sqlite",
    "layout": {
      "catalogs": "catalogs",
      "projections": "projections",
      "repos": "repos"
    },
    "profile": "local",
    "repository": "filegit",
    "secrets": {
      "elasticsearch": "KC_ELASTICSEARCH_PASSWORD or KC_ELASTICSEARCH_API_KEY",
      "gitea": "KC_GITEA_TOKEN",
      "redis": "KC_REDIS_PASSWORD",
      "starrocks": "KC_STARROCKS_PASSWORD"
    }
  },
  "views": [
    {
      "viewId": "analyst-board",
      "revision": 3,
      "sources": [
        {
          "repository": "kr://acme/public/metadata",
          "selector": "refs/heads/main"
        },
        {
          "repository": "kr://acme/org/semantics",
          "selector": "refs/heads/main"
        }
      ],
      "retired": true
    },
    {
      "viewId": "kai-desk",
      "revision": 1,
      "sources": [
        {
          "repository": "kr://acme/personals/kai",
          "selector": "refs/heads/main"
        }
      ]
    }
  ]
}
```

当时磁盘 / 其它命令：

- 登记表 git log --oneline

```text
969c02b archive-catalog
bc0d921 retire-release stable
64ad634 retire-view analyst-board
cfef329 define-view analyst-board
e30ff8b pin-view analyst-board
7a2e832 define-view analyst-board
1de16e9 promote desk -> 09956d653ae5
1f00d1a pin-view kai-desk
972ae22 define-view kai-desk
e445ac8 promote stable -> 90b27ad10a46
84cb52d create-preview 90b27ad10a46b3b9be0357e5ca6dbba4678d9ce109fff4ed4e2e40a359eb4781
dbeafbd promote stable -> f5daa2aac586
4ea47eb pin-view analyst-board
4515657 define-view analyst-board
33059b5 register kr://acme/personals/kai
58725c2 register kr://acme/org/semantics
9ab2976 register kr://acme/public/metadata
ab52bed init kr://acme/catalog
396d4d1 root
```

## CLI 观测里和 Go 套件不一样的地方

1. `--as` 的 `commit --changeset` / `merge` 必须再带 `--repo`，否则 stdout 是 `FORBIDDEN` 而不是 Writer/gate 的码。
2. `read --release` 还要成员仓 `read` 规则；只发 allow `--cmd read-release` 时观测是 `[]`，不是 `FORBIDDEN`。
3. local profile 给每个 FileGit 绑 JSONL。主人对 metadata `append` 会成功并在仓旁写 `streams/*.jsonl`（不进 git）。本走通用 allow 拦住采集员，没有用主人去 metadata 上 APPEND。

自动化对照（不依赖 `/tmp`）：`go test ./scenario -count=1`。