# retrieval/cache

可装配的③消费侧 Snapshot 正文缓存，实现 `knowledge.Hydrator`。设计归属
[`SERVICE_ARCHITECTURE.md` §4.8](../../docs/SERVICE_ARCHITECTURE.md)、
[`STORE_ADAPTERS.md`](../../docs/STORE_ADAPTERS.md) 与
[`PROJECTION_CONTROLLER.md`](../../docs/PROJECTION_CONTROLLER.md)。

Goal：减少固定版本的重复正文回源，并为主动维护提供有界预热。Non-Goals：Reader/Snapshot
持有语义缓存、缓存动态 State observation、替代授权、从索引文档提供正文、全仓预热或缓存 READY
证明、分布式一致性与外部缓存介质。对应不变量为 V-01、C-01、CA-01、CA-02、CA-03、PC-01、D-01、P-01、E-01。

## 读取合同

`ReadMany(repository, commit, objectIDs)` 对唯一 miss 保持一次 `BatchReadStore.ReadMany`；
缺少批量能力时只做这些 ID 的精确 `Read`。`ReadAddress(repository, commit, address)` 调同版本
的完整地址读取。两种读取使用独立 key，避免把聚合对象误当作 Entity 单元。

key 包含 Repository、显式非空 Commit、ObjectID，以及地址读取的 Kind、AspectName、MemberKey。
不解析 HEAD，不缓存缺失或错误，不以旧 commit 的值弥补新 commit 的缺失。回源结果先整批验证
仓、版本与对象/单元身份，再填充。地址匹配保留 Repository 的规则：Kind 可以由 Canonical 单元
确定，但 object/aspect/member 必须一致。

缓存只接受 Repository 返回的 Snapshot 声明值；`knowledge/serving` 的动态 hydrate 和交付权限
处理均在它之后。完整 `KnowledgeValue`（正文、Units、Declarations、Binding 声明、Provenance）
在填充与交付时深拷贝，保留原生整数与 byte slice 类型，防止 caller 的裁剪或 State 替换污染
共享副本。无法按 JSON 计量的非标准正文直接绕过留存，读取语义不变。

## 容量与生命周期

`New(Config) (*Cache, error)` 的零值配置为 64 MiB 计量预算、4096 个正文条目；负上限非法。
`MaxBytes` 计量序列化正文、key 长度与每条 metadata allowance，不是 Go heap/RSS 硬上限。
正文条目与有限的冷预热完成标记共享字节预算。正文按 LRU 淘汰，单项超限直接回源交付，
并计入 bypass。不会按历史 commit 无限保留条目，也不单独维护无界热点集合。

`Stats()` 提供命中、miss、回源次数、淘汰、bypass、预热次数/对象/失败、条目数、标记数与计量
字节的线程安全快照。`SourceReads` 计实际正文回源方法调用次数：每次 `ReadMany`、无批量能力
时的每次单对象 `Read`、每次 `ReadAddress` 均各计一次，包含失败和返回不存在的调用；命中不计。
`Clear()` 释放正文和预热标记，并阻止已发出请求重新填充；累计计数保留。
`Close()` 同时关闭留存和预热；后续读取仍可精确回源。

## 主动预热

`Warm(ctx, repository, commit)` 仅供服务维护 worker 调用。它从当前留存 LRU 提取该仓最近访问
的完整对象或精确 Address，保留各自读取形状，默认每轮最多 128 个独立 key。完整对象分成最多
32 个对象的回源批次；Address 缺少批量回源端口，因此在同一个总预算内逐个精确读取。
`WarmLimit` 不超过正文容量，
`WarmBatchSize` 不超过 1000。它不调用检索、不检查可索引字段、不枚举全仓。

新 commit 以新的 key 填充：即使只变更未索引字段，也能预热新正文；旧 pin 的内容在容量允许时
继续可用。成功只说明这一小批热点已尝试读取，不代表全仓或查询投影 READY。删除对象不会用
旧版本补齐，错误交给维护控制器重试；取消在同步回源调用之间检查。

`ColdStart: true` 时，无热点的仓可以使用 `SnapshotObjectPager` 的首个有界身份页，最多
`min(WarmLimit, 1000)` 个，不追 continuation。同仓同 commit 的成功冷预热在无正文可驻留时
留下有界完成标记，空仓不会每次 tick 重复扫描；标记会因容量淘汰或 `Clear` 丢弃，之后允许重新
尝试。正常驻留正文无需标记，因此这些正文被淘汰后，相同 commit 可以再次冷预热。
没有 pager 的仓只等待真实访问形成热点。缓存预热无需逐对象 invalidate；写入之后的固定新版本
天然使用不同 key，容量清理由 LRU 处理。

```go
bodyCache, err := cache.New(cache.Config{ColdStart: true})
// 装配根将 bodyCache 作为 knowledge.Hydrator 注入消费执行器；
// 服务维护控制器在固定 commit 上调用 bodyCache.Warm(ctx, repo, commit)。
```

## 验证

`go test -race ./retrieval/cache` 覆盖跨仓/版本隔离、完整地址、批量 miss、错误/缺失不缓存、
错误身份整批拒绝、深拷贝、LRU 双上限、预热非索引字段与旧 pin、有限冷启动标记、并发访问及
Clear/Close 与进行中回源的竞争。
`TestClearCancelsWarmBetweenEpochCheckAndReadStart` 还固定验证预热 epoch 检查和实际读取之间发生
Clear 的交错：整次 Warm 保留原 epoch，后续对象批次与 Address 读取均不能获得清空后的回填资格。

本地对比可复现：

```bash
go test ./retrieval/cache -run '^$' -bench '^BenchmarkSnapshotBodyRead$' -benchmem -count=1
```

夹具为约 30 KiB 的固定版本对象，authority 只从已在内存的 JSON 解码，未加入网络、磁盘或
人为等待。`direct_authority` 直接回源；`cold_cache` 包含每次清空、回源、验证、填充与深拷贝；
`hot_cache` 预先填充同一对象，计时不包含首次填充。每项同时报告 `source_reads/op`。
2026-09-08 在 Apple M4 Pro / Go 1.26.0 的一次运行：

| 读取 | ns/op | B/op | allocs/op | source_reads/op |
|---|---:|---:|---:|---:|
| 直接解码回源 | 100964 | 51064 | 402 | 1 |
| 空缓存回源并填充 | 171563 | 165890 | 1452 | 1 |
| 热缓存 | 19894 | 36816 | 442 | 0 |

这组数说明本地回源次数与克隆/填充成本，不代表真实 Dolt/Gitea 的延迟或端到端 SEARCH 提速。
缓存 miss 的计量和填充也有开销，应结合真实命中率与 authority 成本判断收益。
