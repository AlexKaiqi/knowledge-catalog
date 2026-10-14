---
entity: kc/mechanism/governance
aspect: definition
schema_ref: schema/system/explanation/v1
path_hint: mechanisms/governance/definition.md
---
# 变更评审与合并

需要评审的单仓变更先形成提案与固定预览，检查有效证据后再合入目标仓。

## 变更提案（Proposal）

在固定 candidate 上准备变更，形成评审预览与证据，再请求推进目标仓的治理对象。

目标又被修改会使旧证据失效，需要重建有效预览。治理复用 Writer 校验与并发保护，一次提案仍只修改一个仓。

## 合并检查（Gate）

在变更提案合入目标仓之前，检查固定预览与所需证据是否有效的治理约束。

Gate 检查已有证据，不临时运行用户套件，也不成为普通 Writer 提交或读取的前置条件。外部套件结果是调用方断言，记录 PASSED 不代表平台运行了套件。

相关说明：[Writer](../../interfaces/writer/definition.md)、[版本](../../concepts/version/definition.md)、[出站扩展](../hook/definition.md)。
