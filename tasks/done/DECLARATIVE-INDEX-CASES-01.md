# DECLARATIVE-INDEX-CASES-01 · 重构前用例补齐与基线（2026-10-13）

- [ ] 测试先行已补写并取得基线；两条既有后台来源合同仍红，未标记整项验收完成。按 U1～U6 复用字段、范围、分页及 Repo a/b 竞态，补 Dataset 发布失败/取消、查询中途切发布版、账仍 READY 的物理索引丢失恢复及 State 发布失败。只改测试与用例关联、不重构生产实现；仅为验证启动已缓存的隔离 OpenSearch，退出已清理，未安装依赖或提交。
- 所触 owner：声明式索引、索引控制、检索、插件接入及验证体系；场景就近关联沿 `.data/scenes/README.md`。现有 `discovery/` 仍是可执行代码位置，待迁移时随具体实现移入 `declarativeindex/`；本轮不创建仅含假框架或重复合同的目标包。
- 基线：既有 Dataset 准备/发布和 Repo 候选 a → Writer b → 对象回读 a 的定向测试通过，日志 `/tmp/kc-index-cases-baseline-20261013.log`。`tree.py --check-product` 实际失败：文档已改章节/条目格式，仍采用旧标题的产品关联无法解析，先修复关联与稳定 U 标题。测试草稿编译/前态错误只算编写问题，不记为实现红证据。
- 充分/必要边界：既有消费者注册、隔离/等待、scope-before-body 与固定观察断言不机械复制。来源采用到插件登记/真实实例、共享 OpenSearch 物理隔离、说明读取及完整宿主退出缺公开接线，沿现有 gap 具体记录；不写不能执行的 feature、不用测试注册器或换 Projection 注入伪造通过。用例写在本轮，运行结果与未实施插件旅程分开报告。
- 真实红基线：全包 `discovery/service` 的两个既有 `TestProjectionNoticeExecutorPreservesBackgroundSource*` 失败；独立复跑仍失败，日志 `/tmp/kc-index-cases-known-red-20261013.log`。位置为 `discovery/service/operations.go` 的 notice 调用 `SetObservationSource(request...)`，判定为请求覆盖后台来源/身份，拒绝通知也残留覆盖；当场关联 U6，不改/削弱测试，不在测试先行轮顺手修实现。触发处置为插件维护改接前先按 index-control 的后台独立身份合同修复并跑现有反例至绿。
- 新用例证据：新增四个文件的四组测试，含冻结/权威接受失败/接受前取消、Dataset a→b 回读与 scope、物理索引丢失的重启/周期恢复、State 全量/增量候选保存后 Commit 失败及重试。实际 Writer、Index、Controller、Dataset authority、查询服务和对象回读参与，物理端口仅注入故障/候选；新测试普通与 `-race` 全绿且无 skip（`/tmp/kc-index-cases-new-20261013.jsonl`、`/tmp/kc-index-cases-race-20261013.log`）。独立复核收紧 Schema 拒绝的具体错误原因，未新增假插件登记器或为先行用例改生产逻辑。
- 关联与门禁：恢复 owner 的稳定 U1～U6 三级标题及产品章节关联，补现有独立断言和四条新 evidence、后台身份红例与具体 gap；修正旧 Repo/Dataset 独立选用文字。正文 1493 字符。场景产品/工程/共享前态检查、`make check-docs`、`make check-validation`、`make quality`、逻辑与物理 driver 单测通过；日志 `/tmp/kc-index-cases-validation-20261013.log`、`/tmp/kc-index-cases-quality-20261013.log`、`/tmp/kc-index-cases-logical-provider-20261013.log`。运行关联不等于新插件验收。
- 扩展基线：真实 OpenSearch 下 `discovery/projection` 与 `discovery/datasets` 全包通过，`discovery/service` 仅上述两条背景来源反例失败，无 skip；首个验证 run `.validation/runs/20261013T123706Z-878d22c7831a` 在并行文档变更中 sourceChanged，不能当成单一源码状态验证，保留其失败证据并在本轮文字收口后重跑同一选择。完整部署和尚无公开接线的插件采用/生命周期旅程未执行。
- 最终运行：文字收口后相同真实引擎选择重跑，`.validation/runs/20261013T124237Z-046352544505/report.json` 记录 `sourceChanged=false`、无 skip；只有两条既有后台来源反例失败，索引及 Dataset 包通过。新 Schema 错误原因断言收紧后定向 `-race` 再次通过；最终质量日志 `/tmp/kc-index-cases-final-quality-20261013.log` 退出 0。owner/图重读，正文仅补标题前空行后 1498 字符；本轮用例已交付，红基线和缺插件接线的整项合同仍不记验收通过。

## 关闭

2026-10-15 拆分 `TASK.md` 时关闭，未按原条目验收。早于 2026-10-14 对象模型重构的认领，相关代码与设计已被后续重构改写（见 `0759640` 之后的提交），原范围按旧结构写成，不再继续。整体验收并入 `FINAL-ACCEPTANCE-01`，插件相关用例由验收规格 accept-01～13 取代。
