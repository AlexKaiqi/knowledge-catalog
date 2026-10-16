# DEBT-TERMINOLOGY-TRIAL-01 · 核对试跑与动画材料的术语守卫失败

## 债

- 位置：`internal/arch/terminology_test.go`、`ZK-TRIAL-01` 及其场景材料、`.data/animation/brief.md`。
- 信号：2026-10-15 完整架构测试中 `TestObjectIsTheOnlyNameForCatalogContent` 报错；陌生 Agent 试跑的方法名称及动画需求中的术语反例被匹配。
- 判定：试跑方法名不是对象领域名词，存在语境误报；动画材料需逐项区分引用的退役词与实际采用。不是文件下载行为的回归。
- 处置：由材料与守卫所属任务核对语义，修正真正漂移；误报用窄规则及正反例处理，不能删除断言或跳过目录。
- 触发条件：试跑材料交付或完整架构验收前。

## 目标

保留对象术语约束，同时准确区分产品概念与测试方法名称、明确引用的反例。

## 依据

- [核心概念](../../doc/concepts.md)。
- [工程质量闭环](../../doc/engineering/quality.md)。

## 验收

- 实际不合规产品术语仍能被反例守卫拒绝。
- 试跑方法名与明确语境的引用不产生无意义的目录迁移或全局豁免。
- 完整架构测试、`make check-docs`、`make quality` 通过。

## 前置

与 `ZK-TRIAL-01` 的材料维护协同。

## 不做

不绕过词汇守卫，不批量改动无关业务字段。

## 进展

2026-10-15 本轮目标：术语守卫区分试跑方法、产品名称及明确的反例引用，仍拒绝把退役词用于对象概念。
实际依据为 `doc/concepts.md` 的对象定义与不用的词、`doc/engineering/quality.md` 的误报回喂规则；不改设计。
定向先红证据：`.validation/runs/20261015T120349Z-16b97d475fc8/report.json`。
失败包含试跑方法名、品牌分词时间戳和一句明确否定的动画反例。开工前守卫文件已有未提交修改，
按本轮不得修改他人脏文件的约束，先准备独立回归与补丁，等待范围澄清；未回退原文件造红。

## 本轮最终统一门禁（2026-10-15）

统一证据与命令见 门禁索引（`.validation/runs/20261015T120241Z-task-orchestration/final-gates.json`）。最新 `make quality`（含 check-docs）通过；完整架构 648 pass/1 fail/0 skip（术语，归 DEBT-TERMINOLOGY-TRIAL-01）；`make test` 18 pass/2 fail/0 skip（同一 Dataset 文本能力场景及父级，归 INDEX）；`make test-contracts` 的 component 为 2349 pass/67 fail/6 skip，boundary/e2e 未启动。上述四次均源码稳定；合同失败分组见 报告（`.validation/runs/20261015T135236Z-03378168af4a/findings.md`），分别归 INDEX、库存消费债及回环 HTTP 资源调查，不由本任务替其它线修复。

最终相关定向结果及真实部署边界分别保留在任务专项证据与门禁索引：定向通过不替代完整入口，现有 local 栈 readyz 503，117 项真实验收均 error/not_started。全量门禁尚未全绿，依本轮约定保留 doing；未提交或安装依赖，未降低检查阈值或重置已有环境。

## 恢复推进（2026-10-16）

用户已明确允许在分派范围保留既有未提交改动并追加必要修复。本轮重新核对当前概念与质量规则、
旧补丁和现有 diff 后，已在工作区接入 `terminologyContext` 的词级识别，并更正五份试跑材料中的
对象/事实/信息用词；保留现有方法目录名、品牌与明确否定的反例，不豁免整个文件或目录。
十二组窄规则正反例继续检查同文件中的真实违规、无关 JSON 字段与不能证明为品牌的时间戳。

当前定向基线先红：`20261016-terminology/20261016T012008Z-2bf8d18d3ec6/report.json`，
13 pass / 1 fail / 0 skip，源码稳定。实际修改后的最终定向为
`20261016-terminology/20261016T012310Z-95cdf45d1e89/report.json`，14 pass / 0 fail / 0 skip，
源码稳定；均位于 `.validation/runs/`。中途另有一次测试事件通过但源码变化的运行，仅作过程证据。

本轮范围已冻结，保留 doing，等待统一完整架构、check-docs 与 quality 门禁；未运行全架构或
全量场景测试，不将定向通过写成完整验收通过。未提交、未安装依赖。

- 2026-10-16 L0 固定快照门禁：索引（`.validation/runs/20261016T015828Z-l0-snapshot-gates/gate-index.md`）；完整架构 649 pass / 0 fail / 0 skip；统一门禁仍有主线失败与 6 项 live 未验证，保留 doing。

- 2026-10-16 分源门禁：索引（`.validation/runs/20261016-environment-recovery/gate-index.md`）；bf99完整架构通过并保留术语守卫；v3合同仍有主线红、e2e未完成，继续 doing。

## 交付

2026-10-16 收口：术语守卫已按词级上下文区分试跑方法、品牌和明确反例，五份试跑材料中的真实用词漂移已修正；
保留同文件真实违规、无关 JSON、无法证明为品牌的时间戳等反例，不豁免整个文件或目录。

已验证范围：专项定向报告（`.validation/runs/20261016-terminology/20261016T012310Z-95cdf45d1e89/report.json`）
为 14 pass / 0 fail / 0 skip；后续完整合同报告（`.validation/runs/20261016-mvp-unified-043602Z/gates/20261016T044027Z-d94f92fb680a/report.json`）
中的完整架构实际为 676 pass / 0 fail / 0 skip，包含术语上下文正反例及完整对象术语守卫。
相关守卫与试跑材料在来源 `374f90a4…` 和候选 `8667c2f7…` 之间相同；
质量门禁（`.validation/runs/20261016-mvp-priority-closeout/gates/20261016T045451Z-9e0b9c81b729/report.json`）
通过，包含 `make check-docs`，各次运行源码稳定。

前文仅因其它在途失败或条件 live 未验而保留 doing 的记录已由上述本任务范围证据补齐；本任务不承接真实部署、
模型或其它 live 能力验收。未削弱术语断言、安装依赖或提交；任务移动及文档、质量复核统一执行。
