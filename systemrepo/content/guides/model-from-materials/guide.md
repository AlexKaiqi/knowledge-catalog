---
entity: kc/guide/model-from-materials
aspect: guide
schema_ref: schema/system/usage-guide/v1
path_hint: guides/model-from-materials/guide.md
---
# 从资料到模型（建模方法）

这份指南回答"手上有一批资料，怎样变成可发布的对象"，给出推导顺序、每一步的判定依据，以及固定版本样例的核对入口。形成模型后，再进入操作指南执行发布。

## 先辨认资料里的所指对象

拿到 DDL、业务文档、表格或一次口头交接后，先做三件事：列出资料里出现的名词，标出每个名词是谁在用、描述什么、多久变一次，再标出哪些名词其实是同一个东西的不同说法。

同义词要在这一步合并。门店的"小票"、数仓的"orders 一行"、下单服务的"订单号"，在资料里可能都叫"订单"，但所指的东西不同；反过来，同一张表在不同文档里叫不同名字，说的是同一件事。合并的依据是所指对象，不是名称。

## 一个对象，还是相关的两个对象

这件事由建模者决定，系统既不鼓励也不约束。两种写法都成立：

- 同一个对象的多个方面。订单服务 `trade/order-service` 的 `definition` 和 `runbook` 归交易团队，
  `monitoring` 方面由 SRE 团队在自己的仓里发布，三个方面用同一个 ID。
- 相关的两个对象。物理表 `dw.ods/orders` 归数仓团队，业务实体 `trade/order` 归交易团队，
  两者用关系“订单 落在 表”连接。

判断时可以参考三点：两者是否一一对应，生命周期是否一致，负责人关心的是否同一件事。三点都是，
多半是同一个对象；有一点不是，多半是两个对象。表可以重建、分层、分表，业务概念不随之改变，
所以表和业务实体通常是两个对象。

给别人对象的某一部分补充说明（比如给某一列写业务含义），可以写成同一个对象的一个方面，也可以
写成独立对象，用关系指向 `dw.ods/orders#/columns/amount`。

## 切分：切面、成员、普通字段

上节关系指向列内部地址是设计目标。当前可执行的 Relation 端点只表达对象 ID，
尚不表达方面、成员或字段定位符；不要把 `#/columns/amount` 拼进端点对象 ID 来模拟定位。
下面的可执行关系样例都连接独立对象。

先按"谁维护、多久变"切切面。独立修改、独立归因、可能由不同团队维护的部分做成切面，例如结构、说明、质量、口径分开。切面不产生独立读权限，不要拿切面当授权边界。

再看集合里的元素是否需要单独寻址：需要按稳定键单独读写、单独标失效或单独统计的，做成成员；只是一起读写的，留在切面内当普通字段。都提升为成员会让维护粒度变碎，都埋在字段里则只能整体替换。

成员与独立对象还要再分一步：成员身份依附于父对象及其集合；独立对象有自己的业务身份，
供不同任务持续引用、组织或关联。两者都可以单独读写，不能只凭“能独立更新”作决定。
同一类资料也可以有不同建法，选择取决于消费和维护需求，不由文件目录或字段数量决定。

| 资料与需求 | 可采用的表达 | 一次修改的后果 |
|---|---|---|
| 接口说明始终整份审核、整份替换 | 一个方面内的 operations 列表 | 改一个接口也提交完整方面 |
| 同一服务的接口需要按稳定键分别维护 | interfaces 方面的成员 | 修改一个成员，兄弟成员保持 |
| 数据库列被语义模型、作业等反复引用并参与关系 | 独立列对象 | 列有独立身份；表与列的关系另维护 |
| 只交付一张表的字段清单，需要逐列更新 | 表的 columns 成员 | 键在这张表内稳定，不自动成为独立对象 |
| 对同一素材补一份画面说明 | 素材的 vision 方面 | 素材身份保持，说明单独修订 |
| 多次识别任务各有模型、输入与结果生命周期 | 独立分析对象，关联素材 | 各次分析可分别追溯；不是覆盖唯一 vision |

## 连接：关系还是引用

关联本身有业务含义、需要自己的身份与版本、会被单独引用或单独演进时，做成关系对象。只是对象内部的一个指针，例如某字段记录的外部标识，留在正文里即可。

跨团队维护、生命周期各自独立的两件事用关系；同一件事不同团队各写一部分用方面。

关系的粒度也按维护任务选择：一起发布的一组端点可以是一条多端关系；每条关联需要
独立变化与归因时采用多条关系。角色名为 `member` 不会把端点变成对象模型里的 Member。
正文引用与关系若同时表达同一事实，要明确哪一处负责维护，并同步修订，避免两份事实分叉。

## 取值：发布快照还是 LiveAccess

先完成身份和维护单元的判断，再决定值的来源。member 可以静态保存，也可以外部取值；
来自 API 的材料也可以由接入方采集后经 Writer 发布。更新快慢只是成本因素，关键是消费承诺。

| 消费承诺 | 选择与需要承担的责任 |
|---|---|
| 已审核的口径、手册，要按发布版本复现 | 发布快照；修订后再交付新版本 |
| 接受每天或每小时刷新 | 接入方定期采集并发布；说明采集时间与过期判断 |
| 必须知道读取当时的状态，源系统决定本次可见值 | LiveAccess；提供可用源、取值合同、身份准入与失败处理 |
| 下载视频、图片、脚本 | 文件交付；对象描述文件并指向固定范围内的路径 |

一个方面或成员只有一种值来源。要让手写目标和实时观测分别更新，拆成不同单元；
不为每个字段都增加一次外呼。固定 Dataset 修订不冻结外部当前值，复核当次结果使用原观察证据。
接入方负责单元声明，源团队负责取值服务与描述，部署维护者负责处理器和可信身份范围。
完整练习见[使用动态资源值](../use-dynamic-values/guide.md)。

## 仓与交付范围

仓按维护责任与写权限划分，Dataset 按消费者任务选择文件并固定版本。
命名空间表达语义归属，不跟着仓路径变化；不同团队不必把同一个对象拆成多个身份。
关系端点、Schema 和取值描述的依赖要分别核对：依赖能被解释不等于作为教学材料可下载，
引用也不会自动把端点加入 Dataset 或授予权限。

## 定结构：复用还是新建

复用别人的 Schema 时按 ID 引用并固定版本，不需要在自己的仓里复制一份。判断能否复用看结构能否容纳实际形状，不看名字是否接近。只为省一次查找而复制同构 Schema，会让同一含义出现多个身份，后续无法统一演进。

只有现有类型确实无法表达资料时，才设计新的 Schema。Schema 与实例可以同批提交；Schema 不合法时整批不生效。

上述跨仓复用是设计目标。当前 Writer 的实例校验仍要求 Schema 在目标提交可解析；
公开发布练习因此先下载匹配的 Schema 原文并随实例导入目标仓，保持原身份和结构。
这是当前采用路径，不是要求各团队永久维护自己的同构副本；不要靠改名绕过兼容检查。

## 用固定版本样例核对

以下命令供已认证且能消费公开样例的读者使用，读取所列 Dataset 的固定 revision。
先核对这三个修订可读，再逐项比较文件头中的对象身份、切面、成员键与 Schema 引用。
样例展示判断的结果，不能替代对自己资料的分析；不要从文件目录推断身份或照抄样例业务事实。
若指定修订或路径不可读，先核对样例交付和消费权限，不改成 latest 来凑出结果。

```sh
kc dataset show qinghe-analysis --revision 2
kc dataset show autumn-launch-kit --revision 3
kc dataset show store-oncall --revision 2
```

### 同一名词在不同上下文指什么

`qinghe-analysis` 第 2 版的订单概念说明，用 `contexts` 区分门店小票、数仓行和下单服务订单号。
核对各自所指与引用，判断同名是否同物。这是一份概念说明，不表示三个语境中的订单实例都已发布。

```sh
kc files read --dataset qinghe-analysis --revision 2 --path concepts/order/definition.yaml
```

### 业务实体与物理表是不是同一个对象

同一修订中的桂花乌龙商品、商品表有不同对象身份；`recorded-in` 关系的端点分别指向它们。
比较商品说明与表属性的维护对象，再核对关系端点。不要因为商品被记录在表中就把两者归为同一个 ID。

```sh
kc files read --dataset qinghe-analysis --revision 2 --path products/osmanthus-oolong/definition.yaml
kc files read --dataset qinghe-analysis --revision 2 --path tables/products.properties.yaml
kc files read --dataset qinghe-analysis --revision 2 --path relations-product/recorded-in-osmanthus-oolong-products.yaml
```

### 哪些内容需要单独寻址

销售语义模型的 `category` 维度和 `gmv` 度量有各自的 `member_key`，属于同一对象的两个集合切面；
定义里的 `baseTableRef`、`defaultTimeDimensionKey` 则是普通字段。文件头能核对维护单元的区别。
把它们换成自己的资料时，仍先问是否需要稳定键下的独立读写，而不是把所有列表都提升为成员。

```sh
kc files read --dataset qinghe-analysis --revision 2 --path semantic-models/shop.sales/definition.yaml
kc files read --dataset qinghe-analysis --revision 2 --path semantic-models/shop.sales/dimensions/category.yaml
kc files read --dataset qinghe-analysis --revision 2 --path semantic-models/shop.sales/measures/gmv.yaml
```

### 关系与方面如何区分

`store-oncall` 第 2 版中，定义、接口和处置手册采用同一个服务 ID；活动采用另一个 ID，
`affected-by` 关系连接服务与活动。前者说明同一对象的不同方面，后者表达两个对象间需要独立维护的关联。
核对关系自身的身份和两个端点，不把关系正文当成服务的一个方面。

```sh
kc files read --dataset store-oncall --revision 2 --path services/order-service/definition.yaml
kc files read --dataset store-oncall --revision 2 --path services/order-service/interface.yaml
kc files read --dataset store-oncall --revision 2 --path services/order-service/runbook.yaml
kc files read --dataset store-oncall --revision 2 --path campaigns/autumn-2026/definition.yaml
kc files read --dataset store-oncall --revision 2 --path relations-store-ops/affected-by-order-service-autumn-2026.yaml
```

### Schema 什么时候复用、什么时候另建

订单表与上面的商品表属性引用同一份 `schema/table.properties`，对象不同不要求另建 Schema。
读取所选 Schema，比较字段与约束能否解释两个实例。

```sh
kc files read --dataset qinghe-analysis --revision 2 --path tables/orders.properties.yaml
kc files read --dataset qinghe-analysis --revision 2 --path _schemas/table.properties.aspect.yaml
```

`autumn-launch-kit` 第 3 版同一素材的 `definition` 与 `vision` 采用不同 Schema：
前者描述素材文件，后者有识图标签及产生者。比较两份已交付契约与实例，核对新增形状是否需要自己的约束。
这个样例展示了不同契约的结果，不说明作者曾评估过哪些备选 Schema。

```sh
kc files read --dataset autumn-launch-kit --revision 3 --path assets/autumn-launch/cut-15s.definition.yaml
kc files read --dataset autumn-launch-kit --revision 3 --path assets/autumn-launch/cut-15s.vision.yaml
kc files read --dataset autumn-launch-kit --revision 3 --path _schemas/media-asset.definition.aspect.yaml
kc files read --dataset autumn-launch-kit --revision 3 --path _schemas/media-asset.vision.aspect.yaml
```

## 顺序与产出

想从一条练习开始，可读取新增的公开导读：`autumn-launch-kit` r4 的
`assets/autumn-launch/readme.md`、`qinghe-analysis` r3 的 `semantic-models/onboarding/readme.md`、
`store-oncall` r3 的 `services/onboarding/readme.md`。上述固定旧版样例继续成立。

按上面的顺序走，每一步都有可核对的产出：候选实体清单、每个实体的边界判断与理由、切面与成员的分配、关系清单、每个对象采用的 Schema 身份。这些不必发布成对象，但要在提交前写下来，因为它们是后续修订和冲突判断的依据。

整理完再进入 [首次发布领域对象](../first-publication/guide.md) 执行发布。那里讲命令、基点、回执与读回；这里讲发布什么。

## 会错的地方

常见误判包括：没有核对所指对象就混用表结构与业务概念，导致任一侧变化都迫使身份迁移；仅因名称不同就把同一个对象拆开，导致同一含义多处维护；不需要独立寻址却把每个字段都做成成员，导致粒度碎片化。

发现误判后，先核对已有身份、历史实例与消费范围，再决定怎样修订，不能只新建对象而放任旧身份继续表达错误含义。兼容的 Schema 修改必须继续解释已有实例；涉及身份拆分、合并或不兼容 Schema 修改时，先明确迁移影响。现有合同尚未定义不兼容 Schema 修改的迁移方式，不要假设它可用。

所需能力：[理解对象类型](../../capabilities/schemas/definition.md)、[发布与修订对象](../../capabilities/publish-object/definition.md)、[沿关系探索](../../capabilities/explore-relations/definition.md)。
