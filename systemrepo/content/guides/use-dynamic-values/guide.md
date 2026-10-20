---
entity: kc/guide/use-dynamic-values
aspect: guide
schema_ref: schema/system/usage-guide/v1
path_hint: guides/use-dynamic-values/guide.md
---
# 使用动态资源值

适用于读取方面或成员时，其值由外部来源提供的情形。先取得可消费的 Dataset 和明确 revision，或有权读取的来源仓及固定 commit；同时满足 KC 与源侧访问要求。声明可读、处理器已装、本次获权和源可用要分别核对。

## 核对声明与目标

先判断是否需要实时取值：已审核内容用快照，允许周期刷新时由接入方采集后经 Writer 发布，
视频等已发布文件沿文件入口下载。只有任务要求读取当时的状态，且源能承担取值、准入与失败语义时，
再接 LiveAccess。对象/方面/member 的划分与取值方式分别决定。

第一次接入可跟随公开的静态到动态练习。它包含可下载的源程序、Schema、静态成员、
取值描述与替换声明，以及逐值失败和原观察复核步骤：

```sh
kc read 'urn:kc:object:example/store-oncall#/readme' --dataset store-oncall --revision 3
```

这个修订交付练习材料，不表示公开部署已启动练习源。只有部署维护者能确认处理器与网络条件，
源团队负责取值服务和源侧身份验证，单元维护者负责发布引用声明。

```sh
kc plugin liveaccess help
```

按插件说明读取目标单元的取值来源及其引用的 ResourceDescriptor。产物是目标对象 URN 与定位符、固定声明版本，以及描述中的 origin、模式和身份要求。取值来源只引用描述；Schema 只约束值结构，不能用它推断出站目的地或权限。

找不到描述、没有处理器或当前范围不支持时，先修复声明或联系部署维护者；不能自己编造外部 URL、凭证或调用方式。凭证来自部署与获准身份，不写进对象内容。

## 读取所需单元并判断观察

替换明确坐标后读取，插件按声明透明取值。以下以 revision 3 演示，执行时换成实际固定版本号：

```sh
kc read '<object-urn>#/<aspect-or-member-pointer>' --dataset <dataset-id> --revision 3
```

只选择任务所需单元，保留返回的值、逐值失败和观察依据。核对外部来源、实际观察时间、适用身份与 Schema 校验结果；固定 revision 固定的是声明和解释依据，不会把外部当前值冻结成历史值。需要复现当时的外部值时，使用已保存的观察证据，不能靠重新请求同一 revision 假装取回旧值。

部分单元失败时逐项判断是否还能完成任务；源拒绝、超时或值不合 Schema 时保留失败，修正源准入或数据后再读，不能把旧缓存当成本次成功。per-caller 单元按实际调用者取值，不能借别人的观察或共享索引越权；共享索引只接受描述为 shared 的外部单元。

## 保存结果与完成判断

仅需要当前状态时，交付观察值及其依据即可。确需持久保存时，先选出要保存的值，记录源与观察时间，再在有写权的业务仓准备普通对象草稿，经 Writer 显式发布并按接受 commit 读回，流程见[首次发布领域对象](../first-publication/guide.md)。读取本身不写仓，观察也不是仓提交。

完成判据是任务所需值均符合采用的 Schema，来源和观察时效可复核，失败部分未被冒充成功。发布静态副本只固定这次保存的内容，不代表以后仍是最新外部状态。

所需能力：[理解对象类型](../../capabilities/schemas/definition.md)、[访问动态资源](../../capabilities/resource-access/definition.md)、[发布与修订对象](../../capabilities/publish-object/definition.md)。
