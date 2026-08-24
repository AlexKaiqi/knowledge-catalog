# DSH Loom UI 代测记录（2026-08-24）

本记录对应 [DSH_LOOM_OPERATOR_RUNBOOK.md](DSH_LOOM_OPERATOR_RUNBOOK.md)。测试通过 Microsoft Edge 中的 DSH UI 完成；后台只用于核对真实状态和运行黑盒验证，不替代 Agent 操作。

## 测试环境

| 项目 | 值 |
|---|---|
| DSH Owner | `http://127.0.0.1:61261/` |
| DSH Intruder | `http://127.0.0.1:61307/`，`KC_AS=intruder` |
| KC HTTP facade | `http://127.0.0.1:7390/` |
| KC home | `/tmp/dsh-loom-dw/.kc-home` |
| Catalog | `kr://acme/catalog` |
| 默认 Workspace | `analyst-board` |
| DSH profile | `dsh-loom`，GPT-5.6 Sol High |

身份由各 DSH 进程启动时的 `KC_AS` 固定。本轮没有在聊天中伪造身份切换。

## UI 实测结果

### 1. Profile 与 Skill 自动加载：通过

发送业务请求时没有要求 Agent 加载 Skill。DSH 轨迹自动出现 `Skill knowledge-catalog`，并按 Catalog 操作语义执行。界面中可以看到 `dsh-loom` profile。

### 2. Owner 初始化公司拓扑：通过

发送给 Agent：

> 为 Acme 建一个公司级知识 Catalog。挂载三个知识仓：kr://acme/public/metadata、kr://acme/org/semantics、kr://acme/personals/kai；创建 analyst-board（metadata + semantics）和 kai-desk（仅 personal）两个 Workspace。直接执行，不要让我手工加载任何 Skill。最后用系统真实状态核对 Catalog、Repository 和 Workspace。

Agent 创建了 Catalog、三个 Repository 和两个 Workspace。第一次把两个仓都挂在 Workspace 根路径时，系统以 `mount path <root> is claimed by both ...` 拒绝；Agent 随后改为 `metadata/`、`semantics/` 两个显式 mount 路径并成功，没有绕过约束。

后台 `kc read --catalog` 核对到：

- `kr://acme/public/metadata`
- `kr://acme/org/semantics`
- `kr://acme/personals/kai`
- `analyst-board` revision 1：`metadata/` + `semantics/`
- `kai-desk` revision 1：个人仓挂在根路径

本次解析得到的固定坐标：

| Repository / Workspace | Commit / pin |
|---|---|
| metadata | `807909a901cba06e845792dac6a276fee5c3a5a5` |
| semantics | `140ff2fcd8f5cc41485758486979503dc425d255` |
| personal | `d34f6eb85fc22bdb3a40e0182a7c8c9282a8f470` |
| analyst-board pin | `d0c260425e0d2150a2b6e4a1f8a83107a34ef38459d488b44537b92d9a59b0fe` |
| kai-desk pin | `066e26be2a0e629ec6491423d37a2029ccbdca39bfacc3f6da70eb47c81cb611` |

### 3. 只经 Workspace 消费：通过

发送给 Agent：

> 现在只通过 analyst-board 这个 Workspace 做消费核验：解析本次固定仓 commit，列出当前可见知识，并说明 metadata 与 semantics 是否为空。不要直接按 Repository 读取。

Agent 通过 `resolve --workspace` 和 `list --workspace` 消费，结果保持同一组固定 commit；两个仓当前都为空。没有把 public 知识复制进个人仓，也没有把 Workspace 当 Repository。

### 4. 最小角色授权：部分通过

Owner 已实际写入 11 条规则，覆盖以下主体：

- `collector`：metadata 的 `put/remove/commit`
- `steward`：semantics 的 `propose`
- `reviewer`：analyst-board 的 `preview/validate/record-validation`，以及 semantics 的 `merge`
- `analyst-agent`：analyst-board 的 `read-workspace`，以及 metadata/semantics 的 `read`
- `kai`：personal 的 `put/remove/commit`，以及 kai-desk 的 `read-workspace` 和 personal 的 `read`
- `auditor`：Catalog `audit`
- `intruder`：无规则

这一轮没有继续补齐 auditor 的对象 `log/provenance` 权限，也没有完成逐主体逐命令的允许矩阵断言，因此本项记为部分通过。

### 5. 未授权发布必须无副作用：通过

在独立的 Intruder DSH 进程中发送：

> 尝试把一个伪造指标 Metric:gmv-forged 直接发布到 kr://acme/org/semantics 的 main，值为 {"formula":"sum(fake_amt)"}。如果系统拒绝，原样报告错误。不要修改权限、不要切换身份、不要绕过 Writer。

DSH 自动加载 Knowledge Catalog Skill，仅尝试一次，返回原始错误：

```text
Error: intruder is not allowed to put
```

拒绝前后 semantics HEAD 都是 `140ff2fcd8f5cc41485758486979503dc425d255`，证明失败没有写入副作用。

## 自动化佐证

以下黑盒验证通过：

```text
go test ./validation/workbench
ok kc/validation/workbench 17.787s

./validation/playbook.sh DW-03
DW-03 PASS: real binlog update applied once, replayed once, rejected at old position, and changed profile/Q5 exactly
```

`DW-03` 使用真实 MySQL binlog 坐标，证明一次更新被应用、同位置重放幂等、旧位置被拒绝，并且 profile/Q5 只发生预期变化。实际结果在 ignored 文件 `.data/datawarehouse/actual/dw03.json`。

## 暴露的问题

1. 多仓 Workspace 首次创建时，Agent 不熟悉显式 mount 路径语法，失败后花了额外时间查找；`dsh-loom` Skill 应直接给出多仓示例。
2. Workspace 虚拟文件系统与宿主源码文件系统的边界仍不够显眼。消费核验期间，Agent 曾尝试通过 Loom 的 `Read` 打开宿主 `cli/verbs_read.go`，失败后才回到 `kc list --workspace`。Skill 应更强地约束：业务消费只走 `kc`，不要用 Workspace FS 探测宿主源码。
3. UI 在自动化控制下偶尔进入空白/休眠显示，打开开发者工具后可恢复可访问树；本轮没有发现 DSH 或 KC 服务进程退出，因此暂记为浏览器自动化环境问题。

## 结论

核心最小闭环已经成立：profile 可见、Skill 自动加载、Owner 能创建多仓组合、消费者能按固定 Workspace pin 读取、未授权主体被拒绝且无副作用、真实 MySQL 增量验证通过。下一轮应按操作手册继续完成 Collector → Steward → Reviewer → Analyst → Kai → Auditor 的完整角色链路，并把授权矩阵变成自动断言。
