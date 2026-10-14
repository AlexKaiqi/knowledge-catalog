# 对象模块重构 Guideline

## Goal

让人、应用和 Agent 通过 `kc help` 本身就能理解 object_id、Aspect、Member、Address、Relation 等概念及其联系，知道 Object 提供哪些能力；再通过统一的对象 URN＋locator 读取、创建、更新和删除对象知识，取得指定数据并追溯其版本和来源。基础读写可独立采用，统一引用不受文件布局或现行 Address 层级限制。

## Address：统一访问引用与维护定位

READ、PUT、REMOVE 对外只使用一种引用：以对象 URN 为主体，JSON Pointer 放在 fragment，形式示意为 `〈对象URN〉#〈locator〉`；访问语境、版本依据作为明确限定。不保留将 Repo、object_id、aspect、member 拆开传入的另一套寻址。服务解析完整引用，按读取或维护合同计算实际范围；Go `Address` 只表达内部维护坐标，不能代替公共访问引用。

| 坐标 | 选定语义 |
|---|---|
| 对象身份 | 对象 URN 指认来源 Repo＋object_id；路径或交付布局变化不改变身份，不同仓的同名对象不合并 |
| 访问语境 | 可直接读来源 Repo，也可经 Dataset 读取；Dataset URN 选择消费范围，不能替换对象身份中的来源 Repo |
| 版本依据 | 直接访问先将仓引用固定为 commit；经 Dataset 先固定已接受发布版，再采用其来源 commit。另给源版本时必须一致，读取期间不追随 HEAD |
| 内容 locator | JSON Pointer 从对象内容根选择节点，不指向文件路径或交付信封；可访问 Aspect、稳定键的 Member 及任意深层字段 |
| 维护范围 | 服务按对象表示与维护模型解析 locator，确定整体值、Aspect、Member 等实际目标，再形成内部 Address 变更；定位格式统一，支持的写入范围须明确说明 |

### Locator 长什么样

Locator 是从对象内容根开始的 [JSON Pointer](https://www.rfc-editor.org/rfc/rfc6901.html)：每层用 `/` 分隔，对象按键名、数组按从零开始的下标定位。它不包含 Repo、object_id 或版本，也不指向文件路径。

例如，一个按 Aspects 组织、其中 columns 按 Members 维护的对象，组装后的内容是：

```json
{
  "description": "订单表",
  "columns": {
    "order_id": {
      "type": "string",
      "constraints": {"nullable": false}
    }
  },
  "tags": ["交易", "核心"]
}
```

| Locator | 取得什么 |
|---|---|
| 空字符串 | 整个对象内容 |
| `/description` | description Aspect 的值 `"订单表"` |
| `/columns` | columns 集合 |
| `/columns/order_id` | order_id Member 的完整值 |
| `/columns/order_id/constraints/nullable` | 深层字段值 `false` |
| `/tags/0` | 数组第一个值 `"交易"` |

将深字段 locator 拼到引用后，写成：

```text
〈对象URN〉#/columns/order_id/constraints/nullable
```

空 locator 可省略 fragment 或保留结尾 `#`；`/` 则选择空名称字段，不代表对象根。Member 使用稳定键，数组下标只表示固定版本中的位置，没有通配或条件筛选。

键名中的 `/` 写为 `~1`，`~` 写为 `~0`：单个键 `a/b` 用 `/a~1b`，键 `a~b` 用 `/a~0b`；点号是普通字符，键 `a.b` 用 `/a.b`，不拆成两层。先做键名转义，再对 fragment 做 URI 编码，例如带空格的键 `display name` 写成 `#/display%20name`。解析时先拆引用，再做 URI 解码、JSON Pointer 解码；`#/a%2Fb` 解码后是两层 `/a/b`，不能代替单键 `/a~1b`。

直接读取检查仓权限；经 Dataset 读取检查该发布版的当前消费权限及文件、解释材料范围，仍保留来源对象 URN。同名对象不能仅凭 Dataset＋object_id 猜测。Dataset 未选入完整对象时须报告范围或完整性限制，不补读源仓、不冒充完整对象；不存在的节点与合法空值须区分。

访问引用作为请求数据完整传给服务，使 locator 进入读取计划；不能指望 HTTP 地址后的 fragment 自动送达服务。返回保留实际对象身份、版本与选择位置，相同对象身份不代表相同读取。

对象创建、更新、删除同样接受对象 URN＋locator，由 URN 选定唯一 Repo、locator 选定维护目标，经 Writer 执行；Dataset 发布由 Dataset URN 指向交付对象，接受来源版本、范围及布局。源提交成功不表示 Dataset 已采用，发布失败不撤销源提交，Dataset 读权不产生源仓写权。此边界沿[Dataset 合同](dataset.md)。

Aspect 是独立维护的切面。对象整体值与 Aspects、同一 Aspect 的整体值与 Members 不能混用。Relation 独立读写并引用对象，不自动读取或发权。Schema 可选；采用后更新继承约束，Schema 自身必须合规。

## 重构后提供的能力与承诺

| 能力 | 输入与成功结果 |
|---|---|
| 认识对象模型 | `kc help` 本身说明 object_id、Aspect、Member、Address、Relation 及其联系、基础能力和一个用法例子；无需先知道系统仓或概念标识，也无需登录、连接服务或启用搜索 |
| 读取对象 | 对象 URN＋访问依据；交付同版本组装的对象内容、固定版本及维护内容摘要 |
| 读取指定数据 | URN＋locator；只交付所选节点及版本、实际采用的 Schema 与已记录来源依据，支持 Member 和深层字段 |
| 创建 | 对象 URN＋locator、新内容及写入条件；经 Writer 创建相应维护内容并发布为新版本 |
| 更新 | 同一统一引用、固定基点及完整新值；服务确定维护目标，经 Writer 完整替换 |
| 删除 | 同一统一引用明确选定对象或维护范围；经 Writer 删除，历史仍可追溯，不从缺文件推断删除 |
| 校验 | 拒绝表示冲突及违反已采用 Schema 的变更，检查 Schema 演进与引用者 |
| 浏览与理解 Schema | 列出固定范围内的 Schema 对象，解释其结构、字段类型与约束；完整正文沿对象读取取得 |
| 追溯 | 读取历史、差异及已记录来源；结构合规不保证事实真实 |

读取先固定依据、核对所选语境的当前授权与材料范围，将 locator 用于计划必要单元和解释材料，再读取、提取并交付所选内容，减少响应传输。单元内部的字段读取可能仍需加载该单元；缺材料、越界或失败须明确报告。

写入保证单仓原子性、并发检查和同命令同内容的幂等回执。校验或并发拒绝不推进；结果不确定时恢复原命令。直接发布推进目标 Ref，提案待治理合并后发布。

## Affordance 与 CLI

Object 负责有稳定身份的知识表示：组装固定内容、校验通用结构、修改维护单元，以及解释该对象的修订和已记录来源。判断入口归属看它兑现哪项保证；操作了对象、位于现有 Object 包、或不访问外部源，都不足以证明归属。

### 第一项：认识对象模型

`kc help` 本身直接给出 Object 模型概览，解释 object_id、Aspect、Member、Address、Relation 及其联系，列出基础能力，并给出一个串起对象身份、切面、成员和定位的小例子。使用者不必再进入另一页帮助，便能建立基本模型、知道下一步做什么。

模型概览由 Object 提供，统一根帮助装配；静态阅读无需登录、连接 Server 或先知道系统仓及概念标识。具体操作的详细参数、条件和恢复说明由既有叶子帮助提供：

```text
kc help            → 直接理解对象模型、基础能力和例子
kc help read       → 了解具体读取用法
kc help writer put → 了解完整单元写入用法
```

根帮助已装配 Object 提供的本地模型概览；叶子帮助与实际命令来自同一模块，无需新增帮助主题或业务动作。

需要深入读取版本化定义时，再按帮助给出的实际访问路径进入系统知识仓。现有读取方式是：

```bash
kc read 'urn:kc:object:kr%3A%2F%2Fkc%2Fsystem:kc%2Fsystem#/readme'
kc read 'urn:kc:object:kr%3A%2F%2Fkc%2Fsystem:kc%2Fconcept%2Fobject#/definition'
```

读取下列概念时，沿用第二条命令，替换引用中的对象身份。根帮助须先解释表中概念，不能仅列这些标识让初学者自行读取：

| 要了解什么 | 现有概念对象 | 应说明什么 |
|---|---|---|
| Object／object_id | `kc/concept/object` | 业务对象、知识表示及其身份；Repo 与 object_id 如何共同指认知识，路径不决定身份 |
| Aspect | `kc/concept/aspect` | 同一对象的独立维护切面及其与完整读取的关系 |
| Member | `kc/concept/member` | 集合型 Aspect 中按稳定键维护的成员 |
| Address | `kc/concept/address` | 如何定位维护单元，及其与完整访问引用、读取 locator 的区别 |
| Relation | `kc/concept/relation` | 有独立身份的关系记录、带角色的端点引用及维护边界 |

object_id 目前在 Object 定义中说明，没有单独的概念对象。系统仓访问仍按实际服务、认证与权限进行；需要按同一版理解模型时，后续读取带上导读返回的 `--commit`。静态帮助与出版的模型说明保持语义一致，完整系统知识的聚合与出版仍归 SystemRepo。这些命令说明访问路径，不证明某个部署已经出版对应版本，也不从帮助或出版物推断当前插件已装配或调用已获权。

### 核心操作

| CLI | 用途及归属理由 |
|---|---|
| `kc read` | 接受对象 URN＋locator，读取同一固定版本的组装内容或所选节点，同时交付版本及相应维护内容的摘要；身份、组装与维护依据由 Object 解释 |
| `kc writer put` | 接受同一种引用及完整新值，创建或替换相应维护内容；Object 决定表示、Schema 与写入前置条件 |
| `kc writer remove` | 接受同一种引用，显式删除对象或选定维护内容；Object 决定删除范围 |
| `kc log --repo <id> --object <object-id>` | 查询这个对象的修订历史，按对象身份归集变化，路径迁移不换身份；仓级提交历史由 Snapshot 提供 |
| `kc provenance --repo <id> --object <object-id>` | 查询固定版本中该对象各单元保存的来源记录；不递归读取来源引用，也不证明外部事实或查询外部访问审计 |
| `kc schema list`／`kc schema describe` | 列出固定范围内的 Schema 对象，解释结构、字段类型与约束，并展示已记录的声明；Schema 是知识对象，这两项属于 Object 基本能力 |

读取直接交付继续维护所需的固定版本和内容摘要，用户可以读取后更新，不必另跑 `kc resolve`。深字段读取的更新依据对应其所属维护内容，不能拿字段值的摘要替代完整维护单元的摘要；已具备完整新值及并发条件时仍可直接提交，不强制先读。

### PUT／REMOVE 到哪一层

PUT 和 REMOVE 的公共目标都用对象 URN＋locator，PUT 另给完整新值；写入命令身份、目标 Ref、固定基点与并发条件沿 Writer 合同。以下示例假定 description 是完整值 Aspect、columns 按稳定键 Members 维护，不能只按路径层数猜测维护模型。

| 统一目标引用 | PUT | REMOVE |
|---|---|---|
| `〈对象URN〉`，空 locator | 创建／替换以整体值维护的对象；按 Aspects／Members 组合的对象不能用组装结果整体覆盖 | 删除该对象全部维护单元，包括它的 Aspects 和 Members |
| `〈对象URN〉#/description` | 创建／替换 description 的完整 Aspect 值 | 删除该 Aspect |
| `〈对象URN〉#/columns/order_id` | 创建／替换 order_id 的完整 Member 值 | 删除该 Member，其他 Members 保留 |

例如，同一个列成员的 PUT 与 REMOVE 指向完全相同的引用。CLI 只用位置参数传入引用，不保留另一套参数别名；示例 Repo 与对象须在实际服务中按权限创建：

```text
kc writer put 'urn:kc:object:kr%3A%2F%2Fscene%2Fknowledge:table%2Forders#/columns/order_id' --kind Member --command-id put-column-1 --value '{"type":"string","constraints":{"nullable":false}}'
kc writer remove 'urn:kc:object:kr%3A%2F%2Fscene%2Fknowledge:table%2Forders#/columns/order_id' --command-id remove-column-1
```

首次建立 Members 维护模型时明确给出 `--kind Member`；既有目标保留实际维护模型，包括整体 Relation 的种类，不能用输入重解释。PUT 不合并字段：替换 Member 时，新值未包含的旧字段也会消失，其他 Members 保留。locator 已表达 Aspect 与 Member 的位置，调用方不再另拆参数。更深字段可被读取，不表示本轮已选定任意字段写入或删除；既有完整单元更新须构造新完整值并以选定依据检查并发。支持的目标不由现有 Go Address 字段数量或 locator 深度单独决定。

若选定让 `〈对象URN〉#/columns` 删除整个 Members 集合，须明确将范围转换为单批 Member REMOVE 及其并发、Schema、缺失和失败语义；深字段或数组位置删除同样须另定操作效果。本轮不以旧 CLI 的“不存在完整 Aspect 单元”判定这些新目标必然失败，也不借统一引用承诺尚未选定的操作范围。

基本读写不接受分列寻址，也不将它暗中转换为统一引用。CLI、SDK 与 HTTP 将完整引用交给服务，由服务按同一维护模型产生 Writer 变更。定位到由 Members 组成的集合，不因此取得集合整体删除能力。缺失条件、内容摘要、版本基点及 Ref 检查继续成立；整对象删除检查组装内容摘要，历史保留且删除仍须通过 Schema 引用等校验。

每次直接 PUT／REMOVE 是一次写入命令；需要同时修改多个对象或单元时，提交同一单仓 ChangeSet，整批校验后原子发布。

### 协作边界与现有入口

`kc diff --dir` 预览草稿目录相对仓基点的变化，`kc writer commit --dir` 组织目录内容、形成批次并请求单仓知识发布；两者及原写入命令的回执查询沿 [CLI](cli.md)与[应用发布任务](service.md)说明，不列作 Object 基本操作。Object Writer 保留知识变更、维护语义、Schema、并发及原子写入保证，编码后的文件和版本提交由 Snapshot 承担。使用了 Object 能力不能推出整个目录流程归 Object，也不能绕过 Writer 把知识发布当作原始文件提交。

Object 自身的差异能力是对照同一对象在两个固定版本中的内容，已有公开 Reader 合同；当前 `kc diff --dir` 没有提供这一入口。对象差异的 CLI 调用形状须另行落实，不能用目录 diff 冒充已经提供。

基础能力清单不单列 `kc resolve`。现有命令可核对对象是否存在、固定坐标和摘要，内部 Resolve 接口也被目录差异流程使用；这些现状不构成用户必须先定位再读取的理由。READ 已直接交付维护依据，旧诊断入口保留兼容产品形状，不因此新增另一套 URN 选择或写前必经步骤。

`kc binding show` 移出基础 Object 清单。它解析 Bound State 的 origin、runtime、protocol、操作和 ResourceDescriptor，属于 LiveAccess 专属声明解释，是否外呼不改变这一职责。Object 保存、固定读取这些知识及校验通用结构；基础采用不要求理解或装配 LiveAccess。专属解析已归 LiveAccess，既有入口保持兼容。

搜索、关系查询和遍历归 Discovery，外部当前值及操作归 LiveAccess，Dataset 发布与仓级版本管理归 Snapshot，提案审阅、验证与合并归 Governance。关系、Schema 与能力声明本身可经 Object 固定读写，专属语义由使用该声明的模块解释。

表中业务命令与参数沿实际 `kc help <命令>` 与包合同。根模型概览、统一引用读写及深层读取已有真实入口与回归；运行范围及剩余缺口沿用例表记录，不以本文或局部绿色证明完整验收。

直接 Repo 读取固定知识声明；经 Dataset 的统一引用沿既有逻辑读取效果，Bound State 的执行仍由 LiveAccess 装配。实际观察保留自身依据，不冒充知识 commit 下的固定正文或可写维护摘要。没有实例单元的虚拟 Bound Aspect 当前需要范围内的已出版对象类型依据和 Schema；仅按所需依据读取，缺材料、缺 runtime 或 Stream 均明确拒绝，不发明切面或补读范围外材料。

## 重构验收用例

以下十二组固定本次 Object 的验收分母。组内独立失败方式分别执行，前态、Oracle、已有断言、
缺口与移除风险沿[验证用例表](test-catalog.md#215-对象模块重构验收)。已有断言优先复用，
旧 Address 成功不能代替新 URN 入口通过，规格写完也不表示实现通过。

### U1：只知道 kc help 就能认识对象模型

无登录态、Server 配置或系统仓坐标，只读根帮助即理解核心概念、联系、基础能力及一个例子；
不发业务请求或创建文件。再沿真实叶子帮助完成一次读改删走读。

### U2：同一引用用于读写并保持来源身份

READ、PUT、REMOVE 共用对象 URN＋locator。路径迁移不改身份，跨仓同名不合并；版本和
语境冲突明确拒绝。分列寻址即使指向同一对象也须拒绝；不能把定位符误作存储路径。

### U3：读取准确范围并固定依据

完整对象、Aspect、Member、深字段与数组位置返回准确值；空 locator、空键、转义、合法
null 和缺失分别处理。读取中 HEAD 前进，正文、实际采用 Schema 与来源仍保持本次固定依据。
选择范围须提前限制必要单元，响应仅含所选值与必要依据，不能先读无关内容再裁剪冒充按需读取。

### U4：读取后依据完整维护内容更新

READ 直接给出固定版本和所属维护内容摘要，调用方无需另跑 resolve 即可完整更新。
深字段值未变、所属 Member 的其他字段已变时，维护摘要必须改变，旧依据更新必须拒绝。

### U5：独立创建、替换和删除维护内容

分别验证整体值、完整 Aspect 和 Member 的创建、完整替换及删除。省略旧字段不会合并保留，
未选单元与其他对象保持；根删除覆盖所有维护单元，旧版本仍可读。集合／深字段删除尚未
选定，不在本轮承诺内。

### U6：最终表示冲突整批拒绝

无 Schema 时也拒绝整体值与 Aspects、完整 Aspect 与 Members 的最终混用。批内顺序不能
绕过检查；显式移除旧表示再创建新表示可成立。拒绝后版本、原内容和已接受回执保持。

### U7：Schema 可浏览、采用且不能绕过

按固定范围浏览和描述 Schema，名单、结构及已记录约束准确。普通内容可不采用 Schema；
采用后更新继承约束。同批声明与实例共同校验，演进检查已有引用者，引用未解除时不能删除。

### U8：Dataset 消费不扩大授权与材料

消费沿已接受发布版的来源 commit，源 HEAD 推进不偷换版本。部分对象和缺失解释材料不能
补读源仓或冒充完整；额外来源版本冲突拒绝。撤权后旧引用也拒绝，Dataset 读权不产生源仓写权。

### U9：并发、失败及重试不损坏写入

单仓批次原子生效，任一前置条件失败无部分泄漏；仓 CAS 与单元摘要各自验证。同命令同内容
恢复原结果，换内容拒绝；不确定结果不能自动重做。提案只推进候选，次级证据失败不撤销已接受提交。

### U10：历史、差异与来源可独立复核

对象历史按身份归集，删除修订和分页准确；固定两版本内容对照不偷换为目录 diff。来源只反映
本对象已保存的依据，不递归访问 sourceRefs，不把 Git 作者或结构合规当成外部事实证明。

### U11：基础读写无需能力插件

未装配发现、外部访问或治理扩展，固定知识及声明仍可读写；来源离线不触发外呼或阻断基础
采用。声明的专属解释与真实执行留在所属模块。

### U12：实际结构与基础合同一致

Reader 实际返回只读能力，写入只经 Writer；类型、组装和解释只有一份。真实 imports、公开
入口及调用方符合所有权和依赖方向，不能以目录存在或同名转发壳代替实现归属。

## 结构终态

身份、表示及固定读写归 `object/`，类型与组装语义只有一份，读入口实际只读、对象写入仅经 Writer；发现投影、外部当前态和完整系统出版各归所属模块，缓存独立装配。任务规则归所属 `service/`，宿主只装配与管理生命周期。跨模块仅用根或具名公开一级入口，保持单向依赖与内部隐藏。

Object CLI 及其对根帮助、既有命令的扩展全部归 `object/cli/`，该模块唯一维护命令贡献、输入校验、帮助、执行和产品输出；独立 typed SDK 归 `object/client/`。统一 CLI 宿主只静态装配贡献、绑定参数与上下文、注入认证客户端并编码结果，不复制 Object 规则或另列一份命令合同。

Object 保证表示与 Schema，Snapshot 写路径和字节、产生版本并条件推进；编码层转换两类变更，更新保留位置、新建默认分配并可显式指定，路径不成为身份。基础读写不依赖插件，外部当前值不因知识版本被固定，持久化仍经 Writer。

## Non-Goals

不改权威知识单元的持久表示、既有能力或授权；URN／locator 的统一读写寻址及其 CLI 呈现是本次新增目标，不以组织迁移阻挡。按需计划采用 Writer 维护的派生坐标；旧路径索引缺坐标时明确报告能力不足并通过显式重建恢复，不在读取请求中全扫正文猜测。写入仍替换、删除明确维护范围，不新增任意字段 PATCH、组合对象整体覆盖或跨仓事务；插件接入独立。

## 硬性约束 / Invariants

沿[既有不变量](architecture-invariants.md)：稳定身份、固定依据、当前权限、唯一对象写面。

## 选定方案 / 被否决方案

选定统一对象引用、按需内容访问及明确维护写入；否决把定位符限制为现行 Address、用文件提交代替对象校验。

## 接口契约 / 状态机

公开能力沿[Object](../../object/README.md)、[Reader](../../object/reader/README.md)、[Writer](../../object/writer/README.md)、[Object Service](../../object/service/README.md)、[Object CLI](../../object/cli/README.md)与[SDK](../../object/client/README.md)。统一引用解析、计划读、维护映射及 CLI 已落实在这些公开合同，实际断言与未验证范围沿[用例表](test-catalog.md#215-对象模块重构验收)。完成须有同一候选的真实入口、失败后态、受影响合同及文档、质量门禁证据；组件或进程内 CLI→HTTP 绿色不代替独立真实部署验收。
