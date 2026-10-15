---
entity: kc/system
aspect: readme
schema_ref: schema/core/readme/v1
path_hint: README.md
---
# Knowledge Catalog 系统仓（KC 协议仓）

这里发布 Knowledge Catalog 关于自身的对象：模型、操作入口、运行机制、能力说明、使用指南、插件合同、导航关系与带逻辑访问声明的基础 Schema。人和 Agent 从这份导读进入，按要做的事情选择需要的内容。

## 开始阅读

业务调用使用已配置的 kc 客户端，经 Server 认证。读取本导读：

```sh
kc read 'urn:kc:object:kc/system#/readme'
```

已知对象身份时可以直接读取；例如读取对象概念：

```sh
kc read 'urn:kc:object:kc/concept/object#/definition'
```

第一次使用，先阅读 [第一次读取对象](guides/first-read/guide.md)。准备建模时从 [对象](concepts/object/definition.md)、[切面](concepts/aspect/definition.md)、[关系](concepts/relation/definition.md)、[Schema](concepts/schema/definition.md)和 [声明式索引](concepts/index-declaration/definition.md) 开始。做具体工作时直接选择下方使用指南。

## 使用指南

这些指南说明如何使用已有能力，例如读取、发布和修订对象。

- [第一次读取对象](guides/first-read/guide.md) — `kc/guide/first-read`
- [接入一个来源](guides/connect-source/guide.md) — `kc/guide/connect-source`
- [首次发布领域对象](guides/first-publication/guide.md) — `kc/guide/first-publication`
- [修订对象的一个切面](guides/revise-an-aspect/guide.md) — `kc/guide/revise-an-aspect`
- [评审并合入变更](guides/review-a-change/guide.md) — `kc/guide/review-a-change`
- [向消费者交付 Dataset](guides/share-a-dataset/guide.md) — `kc/guide/share-a-dataset`
- [恢复结果不明的发布](guides/recover-a-write/guide.md) — `kc/guide/recover-a-write`
- [使用动态资源值](guides/use-dynamic-values/guide.md) — `kc/guide/use-dynamic-values`

## 能力入口

- [身份与准入](capabilities/identity/definition.md) — `kc/capability/identity`
- [进入和查看 登记空间](capabilities/inventory/definition.md) — `kc/capability/inventory`
- [接入对象来源](capabilities/repositories/definition.md) — `kc/capability/repositories`
- [理解对象类型](capabilities/schemas/definition.md) — `kc/capability/schemas`
- [精确读取与复核](capabilities/read-object/definition.md) — `kc/capability/read-object`
- [搜索对象](capabilities/search-object/definition.md) — `kc/capability/search-object`
- [沿关系探索](capabilities/explore-relations/definition.md) — `kc/capability/explore-relations`
- [发布与修订对象](capabilities/publish-object/definition.md) — `kc/capability/publish-object`
- [组合与采用 Dataset](capabilities/datasets/definition.md) — `kc/capability/datasets`
- [查询与管理授权](capabilities/permissions/definition.md) — `kc/capability/permissions`
- [访问动态资源](capabilities/resource-access/definition.md) — `kc/capability/resource-access`
- [维护查询投影](capabilities/projections/definition.md) — `kc/capability/projections`
- [配置 Hook](capabilities/extensions/definition.md) — `kc/capability/extensions`
- [审计与反馈](capabilities/evidence/definition.md) — `kc/capability/evidence`
- [部署与恢复](capabilities/deployment/definition.md) — `kc/capability/deployment`

每项能力有定义、用法与生成的调用入口切面。入口给出同版本 CLI 命令与 help；完整参数、默认值和输入形状由叶子 help 提供。系统仓解释能力的用途与使用方法。`kc help` 提供命令索引。

## 模型与依据

这一组解释对象怎样表示、存放、组合，以及怎样确定版本和来源。

### 对象模型

- [对象](concepts/object/definition.md) — `kc/concept/object`
- [切面](concepts/aspect/definition.md) — `kc/concept/aspect`
- [成员](concepts/member/definition.md) — `kc/concept/member`
- [关系](concepts/relation/definition.md) — `kc/concept/relation`
- [对象地址](concepts/address/definition.md) — `kc/concept/address`
- [Schema](concepts/schema/definition.md) — `kc/concept/schema`
- [声明式索引](concepts/index-declaration/definition.md) — `kc/concept/index-declaration`
- [Meta Schema](concepts/meta-schema/definition.md) — `kc/concept/meta-schema`

### 仓与组合

- [仓](concepts/repository/definition.md) — `kc/concept/repository`
- [系统仓](concepts/system-repository/definition.md) — `kc/concept/system-repository`
- [登记空间](concepts/registration/definition.md) — `kc/concept/registration`
- [Dataset](concepts/dataset/definition.md) — `kc/concept/dataset`
- [固定消费依据](concepts/pin/definition.md) — `kc/concept/pin`

### 版本与来源

- [版本](concepts/version/definition.md) — `kc/concept/version`
- [来源](concepts/provenance/definition.md) — `kc/concept/provenance`

## 操作入口

这一组解释通过什么入口操作系统；具体用途和步骤见能力说明与使用指南。

- [Writer](interfaces/writer/definition.md) — `kc/interface/writer`
- [CLI](interfaces/cli/definition.md) — `kc/interface/cli`

## 运行机制

这一组解释系统如何保证版本、权限、动态取值、评审、扩展、查询与审计。

- [版本底座（Snapshot）](mechanisms/snapshot/definition.md) — `kc/mechanism/snapshot`
- [身份认证与授权（Principal、Permission）](mechanisms/authorization/definition.md) — `kc/mechanism/authorization`
- [动态资源访问（Binding、Observation）](mechanisms/dynamic-access/definition.md) — `kc/mechanism/dynamic-access`
- [工作分支与条件合并](mechanisms/governance/definition.md) — `kc/mechanism/governance`
- [出站扩展（Hook）](mechanisms/hook/definition.md) — `kc/mechanism/hook`
- [派生投影](mechanisms/projection/definition.md) — `kc/mechanism/projection`
- [访问审计](mechanisms/audit/definition.md) — `kc/mechanism/audit`

## 插件合同与查询声明

[LiveAccess 合同](object-interfaces/live-access/definition.yaml) 是内置 LiveAccess 插件的合同，承接当前状态与已声明操作。插件按对象 Schema 生效，由部署安装处理器，不在运行时追加系统仓。基本对象发现与读取不依赖插件。

[声明式索引](concepts/index-declaration/definition.md) 使用 Schema 字段 access 说明正文与声明字段允许怎样查询，这是唯一书写形式。Schema 的结构校验与查询合同解释分别处理，投影实现与就绪由运行环境维护；声明不自动授权，也不表示外部运行值已经可搜索。

## 版本与范围

这份出版物的源文件在项目的 `systemrepo/content` 目录，部署时随当前 Server 版本经 Writer 发布到实际 system 仓。启动核对，运行读取实际仓；返回的 commit 固定导读、正文、Schema、关系和调用入口。记录读取 commit 可以核对本次采用的依据。CLI 与 Server 应使用对应版本，调用入口中的 help 以本机对应版本为准。

能力说明不表示当前部署已启用、投影就绪或当前主体获权。按调用入口查询实际身份、权限、投影或部署状态，以服务返回为依据。无投影时精确读取仍可使用；关系与检索需要满足各自查询合同。

系统仓由平台版本发布，运行时不可写。已认证默认发现与读取由部署授权声明保证；业务 Dataset 显式选择采用系统文件，并选入解释所需 Schema。领域对象与自己的 Schema 发布到目标业务仓，系统基础 Schema 的副本保持与出版物一致。

## 文件与对象

`concepts/` 解释模型与依据，`interfaces/` 解释操作入口，`mechanisms/` 解释运行机制，`capabilities/` 说明用途，`guides/` 提供使用步骤。主体与权限、声明与观察、工作分支与条件合并放在相应机制中解释；目录划分只组织说明内容，不增加 登记空间 的领域类型。模型、入口与机制说明使用同一种正文 Schema。内容按一对象一目录、一切面一文件发布；`_schemas/` 发布 Schema，`object-interfaces/` 发布插件合同，`relations/` 发布独立导航关系。文件路径用于阅读，frontmatter 中的对象标识与切面名确定身份；相关对象也可以按稳定身份精确读取。关系文件可作为对象读取，图查询需要可用的同依据投影。
