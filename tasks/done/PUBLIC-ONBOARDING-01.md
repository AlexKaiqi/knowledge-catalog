# PUBLIC-ONBOARDING-01 · 公开样例承接建模判断与外部取值练习

## 目标

接入方从系统仓按问题找到判断方法，再从三个公开 Dataset 下载导读与练习材料，
理解对象、方面、成员、关系和取值来源的选择，并核对发布和修订的后果。

## 依据

- [对象](../../doc/design/object.md)：稳定身份、维护单元、关系与 Schema。
- [Dataset](../../doc/design/dataset.md)：显式交付与不可变修订。
- [LiveAccess](../../doc/design/liveaccess.md)：声明、观察、逐值失败与身份。
- [Affordance](../../doc/design/affordance.md)：用途、条件、结果与公开入口。

## 验收

- 教学导读通过公开 Dataset 文件与对象入口读取；旧版不被回填，新版追加发布可恢复。
- 建模对照说明选择依据、替代方案和一次修改的影响；现有固定样例坐标继续有效。
- LiveAccess 练习材料从公开交付下载，经 Writer 发布，核对同声明的新观察、
  静态兄弟单元保留、源失败与源拒绝；不把声明出版说成线上实例已就绪。
- 公开命令守卫覆盖教学材料；受影响 Go 回归、`make check-docs`、`make quality`、
  `make test` 与 `make test-contracts` 执行并如实记录范围。

## 前置

保留 `SYSTEM-MODEL-GUIDE-02` 与 `SYSTEM-EXAMPLE-ANCHOR-01` 的既有成果；
操作指南的既有改动属于 `SYSTEM-GUIDE-EXECUTABLE-01`，本任务只补接入决策与样例入口。

## 不做

不改业务对象身份，不改写已发布修订，不发布到既有真实部署，不引入依赖。
关系指向成员的实现另由 `DEBT-RELATION-MEMBER-ENDPOINT-01` 跟踪。
陌生读者的实际任务试跑与进程内合同分别记证据，后者不冒充前者。

## 实现与复核（2026-10-20）

- 系统仓按接入意图导航；建模指南增加字段/成员/独立对象、关系粒度、快照/周期发布/LiveAccess、
  仓与 Dataset 的判断与替代方案。与对象设计同源的边界段保持逐字一致。
- 原 world.json 发布序列完整保留，仅追加 10 步。qinghe-analysis r3、autumn-launch-kit r4、
  store-oncall r3 分别交付公开导读；最后一个还交付 LiveAccess 练习原件。
- 三仓复用同一 readme Schema 导入目录，回归核对它与系统原文一致。现行 Writer 的跨仓
  Schema 采用缺口登记为 `DEBT-WRITER-CROSS-REPO-SCHEMA-01`；关系成员定位缺口登记为
  `DEBT-RELATION-MEMBER-ENDPOINT-01`。保留设计目标，公开教程不冒充已经支持。
- 真实下载的 Python 源与声明经 Server/Writer 练习静态转动态、源变值、503、403、恢复、
  静态方面保留、旧提交读取和原观察审计；读取与审计权限独立验证。
- 同类教程命令沿既有命令守卫检查，不另造命令解析器。旧 Schema 复用回归补齐显式 revision，
  保持文件读取、对象读取、describe 的同一依据；库存回归跟随追加后的当前修订，
  固定成员授权与实际读取采用同一修订，保留撤权和范围断言。
- 基础公开样例未选索引声明，系统导读已改为先精确读取，并明确搜索的声明与就绪前提。
  系统内容检索仍由 `SYSTEM-SEARCH-ENTRY-01` 跟踪，本轮提供意图导航。

## 验证记录

`.validation/onboarding-20261020/targeted-final.jsonl`：168 pass、0 fail、0 skip，
涵盖公开导读所有固定消费命令、完整下载练习、原公开例子、发布指南、建模指南、命令与场景守卫。
`.validation/onboarding-20261020/object-system.jsonl`：对象与系统仓完整包回归，772 pass、
0 fail、1 skip；跳过项是需真实 lakeFS 环境的 Writer 上传吞吐用例。
`.validation/onboarding-20261020/quality-frozen-final.log`：固定候选的 `make quality`
（含 `make check-docs`）通过。
场景 product/views/states 检查通过；原发布序列与共享设计段的独立比较通过。

因工作区同时有其它修改，在固定候选执行完整入口；运行报告均记录 sourceChanged=false。
`make test`：18 pass、0 fail、0 skip，运行 ID 为 `20261020T085731Z-d8ea69e3921b`。
第一轮 `make test-contracts`：5122 pass、1 fail、14 skip；唯一失败是库存回归仍期待旧修订，
已修正并定向复验通过，随后重跑完整合同。
`.validation/onboarding-20261020/candidate-files.json` 记录本轮 24 份实现文件与测试候选逐一一致。

最终 `make test-contracts`：5123 pass、0 fail、14 skip，运行 ID 为
`20261020T091205Z-9b836fc8167f`，状态 passed-with-skips，sourceChanged=false。
完整报告保存在 `.validation/onboarding-20261020/frozen-runs/`，
摘要及逐项跳过清单在 `.validation/onboarding-20261020/summary.json`。
跳过项为既有外部 Luna、Taihu、HTTP runtime、真实 lakeFS 和吞吐/规模用例；
没有新增或修改 skip，也未将未执行项计入通过。

实现与本轮定向验收完成；完整合同仍有 skip，按任务规则保留在 doing。
真实部署发布与陌生接入方任务试跑未执行；工作区未创建提交。

## 交付（2026-10-21）

实现范围：系统仓按接入意图导航；建模指南增加字段/成员/独立对象、关系粒度、
快照/周期发布/LiveAccess、仓与 Dataset 的判断与替代方案；三个公开 Dataset 分别交付
公开导读与 LiveAccess 练习原件（qinghe-analysis r3、autumn-launch-kit r4、store-oncall r3）。

已验证范围：定向 168 pass / 0 fail；对象与系统仓完整包回归 772 pass / 1 skip；
`make quality`（含 `make check-docs`）通过；`make test` 0 FAIL / 0 SKIP；
`make test-contracts` 记录 `20261020T091205Z-9b836fc8167f`，`passed-with-skips`、
`sourceChanged: false`、5123 pass / 0 fail / 14 skip。

当前源码统一门禁 0 FAIL / 18 SKIP，18 项经逐条核对均为既有环境分级项。
真实部署发布与陌生接入方任务试跑未执行，属独立入口，按各自合同单独进行。
