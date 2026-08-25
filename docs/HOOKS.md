# Hook：薄的出站扩展点

日期：2026-08-25

Hook 解决“某个 `kc` 动作前后，平台怎样通知或调用用户系统”。它不是权限、采集器或 merge 证据。具体命令、配置与投递格式以 `hook/README.md` 和代码为准。

---

## 1. 问题与方向

知识底座需要接入 CI、通知和领域检查，但不能把每个业务脚本写进核心协议。方向必须先分清：

```text
allow      决定谁能调用动作
Collector  从外部读取并显式写知识
hook       平台在动作 pre/post 调用户系统
gate       merge 时检查已有证据
```

Hook 是出站调用；用户系统写回 ValidationReport 是 Gate 的入站证据，两者不是同一 phase。

---

## 2. 第一性原理

### 2.1 扩展点必须薄

核心只定义调用时机、最小输入和失败语义，不解释脚本在检查什么。领域规则、审批流和套件实现留在外部。

### 2.2 pre 只能机械否决

pre 可以放行或拒绝整条命令，但不能修改 ChangeSet、补 provenance 或用非确定性模型生成事实。超时应 fail closed，且不得产生部分提交。

### 2.3 post 不能回滚既成事实

post 发生在 Receipt 已持久之后，适合通知、重建投影或触发 CI。失败只能进入重试/outbox，不能撤销已成功的写入。

### 2.4 重放不能重复制造外部效果

同一 `command_id` 的 `REPLAYED` 不应再次触发 Hook。Hook 内也不得递归调用 Writer；派生重算必须成为独立命令和独立来源链。

### 2.5 读路径保持可重放

READ/SEARCH 不挂出站 Hook。可见性由 allow 决定，动态外部读取由 Resource Binding 决定。

---

## 3. 最小语义

Hook 只需要现有动作上的 `pre` / `post` 生命周期点，以及本地进程或 HTTP 等投递实现。

- `pre` 输入只含判定所需的身份、资源、Address/digest 等摘要；成功才允许动作继续。
- `post` 默认只发动作、主体、仓/Catalog、新坐标和 Receipt 等指针；正文由接收方在授权下回读。
- 一条命令触发一次，而不是每个 Address 触发一次。
- 外部管理的 Repository 直接 push 不经过 Writer，因而不会产生 Writer Hook；这是权威边界，不应伪造成已观察事件。

进程内 `catalog.Hook.AfterSnapshot` 是 index sidecar 的内部接缝，不是本文的用户出站 Hook；两者不共享配置或交付承诺。

---

## 4. 与 Gate 的边界

允许：`post-propose` 触发 CI，CI 对精确 Preview 运行检查并写回 ValidationReport。

禁止：把 `pre-merge` 的 exit 0 当作 Required Check。pre 可以是额外否决，但不能替代绑定 Preview 的 Gate 清单。

---

## 5. 决策

- **H-01**：Hook 是 CLI/facade 边界的出站能力，核心 Writer/Catalog/Repository 不依赖用户 Hook。
- **H-02**：pre 只允许整命令放行/拒绝，不允许 mutation。
- **H-03**：post 失败不回滚，必须可重试。
- **H-04**：REPLAYED 不重复触发外部效果。
- **H-05**：读路径、Collector 和 Gate 不建模为 Hook。
- **H-06**：业务目标、脚本和 URL 是部署配置，不是知识对象。

---

## 6. 具体协议位置

- `hook/`、`hook/README.md`：dispatch、exec/HTTP、outbox 与配置。
- `cli/`：动作生命周期接缝。
- `docs/GATES.md`：Required Checks 与 merge 证据。
- `docs/CONNECTORS.md`：外部访问和 Collector。
