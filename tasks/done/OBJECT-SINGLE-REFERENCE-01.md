# OBJECT-SINGLE-REFERENCE-01 · 对象基本读写只保留统一引用（2026-10-14）

- [ ] 本轮认领：按用户最新要求，READ／PUT／REMOVE 只接受对象 URN＋locator，不保留 Repo／object／Aspect／Member 分列寻址兼容；Dataset 范围、版本、Schema、来源、并发和回执是不同职责的限定，不删除。内部 Address 与批量 ChangeSet 仍是执行合同，不冒充第二套基本寻址入口。
- Owner：Object Guideline 的统一引用与基本能力、Object CLI／SDK／Service 公开合同、CLI 模块所有权。用户最新决定覆盖原文中的旧输入兼容承诺；只切基本 read／put／remove，不扩张到治理、Schema、历史或批量目录任务。
- 当前：CLI／SDK／HTTP／Invocation 及 Object 任务服务的基本 READ／PUT／REMOVE 均已删除分列入口，拒绝用例核对无读取／无写入；真实 CLI、HTTP、SDK 与场景调用已迁移，保留原范围、权限、Schema、CAS、幂等及后态断言。根引用已复用既有完整对象缓存，并保留范围前检、撤权、历史版本及非空 locator 按单元读取。曾因 CLI-OWNERSHIP-CLEANUP-01 同时收尾而暂停，但新增拒绝用例要求旧实现先切换，等待其整套验收会形成依赖反转；本项先完成切换，不覆盖其所有权清理与专项入口改动。实现与定向组件已稳定，尚待同一候选完整验收，不能计为已验收。
- 验证前态：旧共享 OpenSearch 已达分片上限，原失败记录保留，未删数据或提高限制；使用已有缓存镜像创建本轮独占临时验证实例，验收后删除该实例，原服务与数据保持。定向实际调用仅发现两处迁移遗漏，已修正其真实输入／提示断言，仍待统一复验。
- 已发现的边界：业务 MR／CI 可自行承担审批；普通 Writer 不因检查结果账而增加必经流程。Repo 合并不自动采用到 Dataset；Dataset 发布者明确选择新的来源版本，服务负责校验及原子接受新发布，历史发布保持。现有 controlplane 的独立核心必要性未被证明，治理入口去留／归属是另一个设计裁决，本项不擅自删除。
- CLI 形状收口：基本读写只保留位置对象引用，删除重复的 `--reference` 参数写法；位置绑定由 Object CLI 自己贡献，宿主只装配，不引入通用命令框架。SDK／HTTP 仍用唯一 `reference` 请求字段。
- 结构审查：完整质量闸原红为 Object CLI 不稳定度 0.80→0.75。独立按原 depth=2、macOS 政策重算已保存的实际改前 import 图与当前图，82 单元及传播成本 15.356820% 不变；单引用切换删除 Object CLI→Snapshot 的旧 ChangeSet／默认 Ref 依赖，默认及维护解释仍由 Object 服务承担，无新增边。其余三条根 CLI 归位删边与已有消费者小幅漂移如实登记。按既有人工结构审查先例接受必要删边，显式 `-write-baseline` 登记较少直接依赖，不改工具／政策／阈值／断言，原红及审查保存在 `.validation/object-single-reference-20261014/coupling-review/`；仍待最终质量闸，不冒称此前无漂移。
- 联合验收安排：根 CLI owner 已经由用户明确通知本聊天，负责稳定候选的 `make test`／`make test-contracts`／`make quality`／`make check-validation`；本聊天只做对象切换的必要定向、race 与实际部署 DFS，不重复整套。基本寻址及别名拒绝定向完成后，本聊天先使用独占 OpenSearch 完成必要验证，再在正常评论中明确交接使用；端点 `http://127.0.0.1:64113`，实例 `kc-object-reference-validation-20261014`，原引擎和数据保留，联合验收完成后由本聊天清理临时实例。正式对象证据保存在 `.validation/object-single-reference-20261014/final-v1/runs/`，同一候选执行期间不再写源码／说明／任务状态，最后交付注记单独核对等价性。
- 完成条件：旧基本寻址及 CLI 引用别名真实拒绝且不产生 I/O／写入；统一引用读改删、固定来源回读、Dataset 范围、当前授权、Schema、CAS、幂等和失败后态全部保留。实际调用、说明及断言同源迁移后，执行受影响合同、必要 race、make check-docs、make quality；同一候选取证，不安装依赖、不提交。

## 用户叫停，交给 DSH 接续（2026-10-14）

本聊天的实现与验收已停止；本项仍为 `[ ]`，没有提交或推送。Object／插件共同层／声明式索引之前完成的重构不重启；这次待收尾的是基本对象读写删除旧写法，以及 controlplane 的职责判断。

1. **已有改动**：基本 CLI 只收位置对象 URN＋locator；SDK／HTTP 只收 `reference`，旧分列输入和 CLI `--reference` 别名均拒绝。Object 任务服务也删去旧读取分支，底层 Address／批量 ChangeSet 保留。完整对象读取已接回原缓存，深 locator 按单元读取。实现、消费测试、场景、帮助和当前手册已迁移；位置绑定与规则仍在 `object/cli/`，根 CLI 只装配。
2. **最后修复**：实际 HTTP／CLI race 中 47 pass／1 fail，失败是缺引用提示；已将 Invocation read／put／remove 统一为要求对象 URN。继而恢复 put／remove 的 command-id 前检，在正文／文件／仓读取之前拒绝；原读写错误旅程及 Invocation 全包已通过。Bulk 用例补合法 command-id 并核对确实达到 Bulk 拒绝分支。此后未取得最终候选的完整 race／合同，不能勾选完成。
3. **先定最终候选，再验收**：聊天“整理项目架构图”（`01a13830-81ae-7b01-9e08-fbd4ae844eeb`）同时完成根 CLI 清理和用户追加的架构设计／结构图／离线手册工作。其曾负责共同候选的 `make test`、`make test-contracts`、`make quality`、`make check-validation`；DSH 接续前只读核对其实际进度，避免覆盖在途改动或重复全套。`be360b0c...` 的 371 条受影响 race 无 fail／skip，quality／validation 也曾同源通过；后续代码提示修复及该聊天的文档／阅读入口修改已形成新候选，这些旧绿只作前态。
4. **剩余执行**：最新候选复验实际 HTTP／CLI 读写旅程、旧输入无请求／无副作用、权限／范围／固定版本／Schema／CAS／幂等／回执恢复及必要 race；与根 CLI 工作一起完成上述完整出口。真实 lakeFS 部署 DFS 单独执行，不能用夹具替代；保留全部原断言，skip 或中断不计通过。证据在 `.validation/object-single-reference-20261014/`，尤其 `final-v1/runs/` 的原失败和 `missing-reference-repair.jsonl`、`invocation-final-preflight.log`。
5. **真实部署执行位置**：宿主直接运行曾因 MinIO 签名上传地址 `minio:9000` 在宿主不可解析而失败；必须使用已有部署网络的 scenes 执行器。可参考 `.validation/object-single-reference-20261014/run-live-deployment.sh`（已有缓存 Go 1.23.6 镜像与依赖、限制编译并发、不启动依赖服务）。最近 run `20261014T041755Z-cd56ba0046de` 被用户叫停：6 pass／0 fail／2 原合同人工走查 skip，整体 interrupted 且 sourceChanged=true，绝非部署验收通过。运行容器与验证进程已停止，日志保留；中断可能留下本次验证的临时 native 仓，按实际唯一名称和既有合同核对清理，不动其它仓。
6. **环境交接**：本聊天的临时 OpenSearch `kc-object-reference-validation-20261014` 保留给接续者，端点 `http://127.0.0.1:64113`，已连接 `kc-deploy-local_default`，当前没有本聊天的验证占用；接续者与其它验收 owner 协调使用，结束后清理这一实例。原 `kc-deploy-local-opensearch-1` 已满分片，原服务／数据卷保留，不能删数据或抬限制换绿。普通测试用现有 Go 1.26.0；质量工具使用缓存 Go 1.23.6 并同步 PATH／GO，原跨版本 UI 307／301 差异沿 CLI 条目的存量债，不削弱断言。
7. **controlplane 待裁决**：现存目录维护候选变更、固定组合、检查结果记录和条件合入；它不是 `declarativeindex/control/` 的残留，本轮没有删除它。独立判断是无需由 KC 托管业务 MR／CI／审批；需要时提供可选 MR 接入，Object 负责格式／Schema／权限／并发约束，原生版本合入由 Snapshot 承担。Repo 合入后，Dataset 发布者明确选择新来源版本并发起新发布，KC 负责校验和原子接受；源接入方只有同时负责这个 Dataset 时才替它发起采用。先按用户这条边界明确必要行为和验收，再决定移除／收口现有控制流程，不先造另一套审批框架。

## 关闭

2026-10-15 拆分 `TASK.md` 时关闭，未按原条目验收。实现已落地，后续对象模型重构在其上继续；整体验收并入 `FINAL-ACCEPTANCE-01`。controlplane 待裁决已由 `fa4f770` 删除 controlplane 了结。
