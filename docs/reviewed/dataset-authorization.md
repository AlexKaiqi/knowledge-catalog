# Dataset 授权

定位：整理稿。**还不是文档图节点**，不能覆盖 [`PERMISSIONS.md`](../PERMISSIONS.md)。验收通过后再升格。

本稿只定义 **谁 × action × 资源**。Dataset 的文件清单与版本见 [`dataset.md`](dataset.md)，不在此重定义。

系统不按接入方或内容类型分流授权。任意两个 Repository 同质。

---

## Goal

在可授权资源上增加 **Dataset**（一份已发布文件清单）。消费通道的读权是对该资源的 **`file.read`**，不是对成员仓的 `knowledge.read`。维护通道仍挂在 Repository 上，两条互不蕴含。

## Non-Goals

- 不定义 Dataset 成员形状、版本或索引（[`dataset.md`](dataset.md)）。
- 不把 path / `object_id` / Aspect 做成 allow 表里的独立资源。
- 不把墙外 SQL / 作业、Store 远程 ACL 写进 `kc grant`。
- 不采用「源仓 deny 在消费通道一律作废」的宣传口径：维护通道仍评仓；消费通道评 Dataset，且 Server 只打开清单内 path。
- 不做对象级 Git ACL；不用交付链抠 Aspect 当保密。

## 硬性约束 / Invariants

- 权限是 `principal × action × resource`。resource 只能是 Catalog、Repository 或 Dataset。
- 消费者日常规则：`file.read` × 该 Dataset。不蕴含任何仓上的 `knowledge.*` / `writer.*`。
- 仓上的 `knowledge.read` 不蕴含 Dataset 消费；Dataset 的 `file.read` 不蕴含整仓读、clone、清单外 path。
- 挂项（发布下一版）：发布方必须已能读每个要挂的 source 文件（仓级读或该仓已认证默认可读）。挂不上则该 path 不得进入 `vN`。
- pin / 版本坐标不锁权限；撤 `file.read` 后即使用旧 `vN` 精确读也失败（现行 `KS-02` 的「pin 不锁未来权限」保留，作用对象从知识集改到 Dataset）。
- SEARCH 候选必须落在该 Dataset **当前服务版** 的文件范围内；不得搜出清单外对象再剥正文。
- 精确 READ 只组装清单内文件对应的 Address；缺的 Aspect 是「视图中无此文件」，不回源仓补未入包文件。

## 选定方案 / 被否决方案

- 选定：可授权资源扩展为 Catalog + Repository + Dataset。
- 选定：消费 action 挂在 Dataset 上，主动作是 `file.read`（打开清单内 blob，并在这些文件上解释知识）。另需 `dataset.manage`（改清单、发版）和 resolve（解 `latest` / `vN`，可并进 consume 或单独）。
- 选定：仓上现行 `knowledge.*` / `writer.*` / 仓级 `file.read` / 治理动作不变，只服务 `--repo` 维护通道。
- 选定：不想暴露的 Aspect = 不写入 Dataset 清单，不是一人一行 path grant。两套对外形状 = 两个 Dataset，或拆仓。
- 否决：消费再要成员仓 `knowledge.read`；`file.read` 与 Dataset `file.read` 双轨；allow 打到每个文件；用 Dataset 授权去 invoke 源系统。

升格时改写现行 `KS-02` / `AUTH-02`：仓名单入集仍不发整仓 READ；**Dataset 上的 `file.read` 足够读清单内文件**。未升格前测试与场景仍按现行「consume 不放行 knowledge.*、正文逐仓 knowledge.read」执行。

## 接口契约 / 状态机

```text
维护：  principal × knowledge.*|writer.* × Repository
挂项：  发布方必须已允许读 source 文件 → 才可写入下一版清单
消费：  principal × file.read × Dataset
        → Server 只打开该服务版（或指定 vN）列出的 path
        → Serving/SEARCH 只解释这些文件
墙外：  源系统自己的三元组（不在本表）
Store： Server 身份碰远程（不在本表）
```

公开动作名升格后由 [`PERMISSIONS.md`](../PERMISSIONS.md) 拥有；grant 规则字段仍由 allow 策略合同拥有。`grant add` 需能把 `--dataset` 与 `--repo` / `--catalog` 作为资源范围（形状升格时选定，本稿不冻结 argv）。

参考实现：`cli/allow.go` 求值；升格后 Dataset 范围规则与仓规则分表项。

---

## 1. 为什么消费不是 knowledge.*

`knowledge.read` 的资源是整张 Repository 版本图。Dataset 的资源是文件子集。用仓动作冒充切片，不是少写一个 grant，而是评错了资源。

知识解释发生在 **已经允许读到的文件上**，不再单独要仓级知识权。`--repo` 读仍走 `knowledge.read`。

## 2. 和 lakeFS RBAC

湖仓：创建者要对 source `fs:ReadObject`；消费者对 Dataset 名做 `fs:ReadObject`。同构到 KC：挂项要读得了 source；消费者 `file.read` × Dataset。

不抄：把 Dataset 名当成可 clone 的 repository ARN；读时完全不评维护通道。KC 消费者根本没有仓权，也没有 clone。

## 3. 升格后对现行合同的替换

| 现行 | 目标 |
|---|---|
| 可授权资源：Catalog、Repository、知识集 | 加上 Dataset，知识集不再作为第三种消费资源 |
| `file.read` 进组合面，不发 `knowledge.*` | `file.read` × Dataset 即清单内文件可读 |
| 正文交付按仓 `knowledge.read` | 消费通道按清单；维护通道仍按仓 |
| `file.read` 含进 consume 又常要仓进 VFS plan | 消费 VFS 只评 Dataset；仓级 `file.read` 只服务 `--repo` |

升格时改 [`PERMISSIONS.md`](../PERMISSIONS.md)、[`ARCHITECTURE_INVARIANTS.md`](../ARCHITECTURE_INVARIANTS.md) 的 `KS-02` / `AUTH-02`，以及 grant 场景。未升格前不得用本稿让现有探变绿或 skip。
