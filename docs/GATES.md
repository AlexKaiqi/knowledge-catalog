# Gate：治理跃迁的证据清单

日期：2026-08-27

Gate 回答“这个精确候选是否具备发生治理跃迁所需的证据”。它不是出站调用，也不是权限。具体命令、配置和检查接口以 `gate/README.md` 与 `controlplane/` 代码为准。

---

## 1. 问题

一次 proposal 可能需要结构检查、领域契约、owner 审批等证据。如果 merge 临时调用外部脚本，候选移动、服务离线或脚本变化都会让结果不可重放。

因此 Gate 不在 merge 时“拨电话问是否可以”，而是检查一份已经绑定精确 Preview 的证据清单：

```text
allow  → 谁可以请求 merge
gate   → 这个 Preview 所需证据是否都已 PASSED
hook   → 可选地触发 CI 或做额外机械否决
```

无权与证据不足必须是不同失败。

---

## 2. 第一性原理

### 2.1 证据必须绑精确候选

Review、Validation、Approval 和 MergeGate 不能只绑分支名。ValidationReport 必须绑定完整 Preview：Workspace pins、候选 overlay 和内容摘要共同形成不可混淆的 basis。

Candidate、目标 Ref、Preview 成员或内容变化后，旧证据不得沿用。

### 2.2 核心只理解证据形状

底座只需要检查：检查身份、PASSED/FAILED、Preview basis。它不解释 `domain-contract` 或 `approval:steward` 的业务含义，也不托管套件实现和审批产品。

### 2.3 Gate 只拦治理跃迁

COMMIT 的结构和来源约束由 Writer 同步执行；把外部 CI 变成每次采集的 Gate 会破坏写入可用性并诱发旁路。

读路径也不设 Gate。读者跟 Workspace 已发布 selector；merge 推进成员仓 Ref 后，下一次解析自然看到新版本。

### 2.4 记录结果不等于运行检查

`record-validation` 是证据写入口，不运行用户套件。内建结构检查只证明协议结构，不应冒充业务口径验证。

---

## 3. 最小模型

```text
Preview = pinned workspace + candidate overlay + preview digest
ValidationReport = check identity + outcome + Preview basis
GatePolicy = transition + required check identities
```

merge 的判定是：调用者通过 allow；目标 Repository 对本次跃迁要求的每项检查，在同一 Preview basis 上都有 PASSED；随后才允许 CAS 推进目标 Ref。

Hook 可以触发产生证据的 CI，也可以额外否决 merge，但它不能提供或替代 Required Check。

---

## 4. 决策

- **G-01**：Gate 是纯检查，不在判定时调用外部系统。
- **G-02**：所有证据绑定精确 Preview，不绑定可移动分支名。
- **G-03**：核心只解释检查身份、结果和 basis，不解释套件内容。
- **G-04**：Gate 只用于 proposal → published 的治理跃迁，不用于 COMMIT、动态观察或 READ。
- **G-05**：allow、Hook 与 Gate 分别表达主体授权、出站扩展和候选证据，不相互替代。
- **G-06**：业务 Gate 配方属于部署/场景配置，不是 Knowledge Repository 内容。

---

## 5. 具体协议位置

- `gate/`、`gate/README.md`：清单与纯 `Check`。
- `controlplane/`：Preview、Validation basis 与 Merge 顺序。
- `docs/HOOKS.md`：出站触发与额外机械否决。
- `docs/PERMISSIONS.md`：谁可以请求 merge。
- `docs/WALKTHROUGH_v5.1.md`：当前操作流程。

最小公开操作路径：

```bash
kc gate-add --on merge --repo kr://acme/public/core \
  --require validate,suite:approval:steward
kc validate --preview <preview-id>
kc record-validation --preview <preview-id> \
  --suite approval:steward --outcome PASSED
kc merge --proposal <proposal-id> --preview <preview-id>
```

`require` 是一条逗号分隔的完整清单。存在匹配 Gate 时，`merge` 从已保存的
Proposal 推导授权所需的目标 Repository/Ref，并检查该 Preview 上所有已保存的
证据；调用方不重复传 Repo，也不传 validation ID 数组。成功回执返回
`repository`、`targetRef` 和 `gate {status,basis,required}`，它就是公开证据，
无需读取 `control.json` / `gates.json`。没有匹配 Gate 时，兼容路径仍要求一个
PASSED 的 `--validation <id>`。
