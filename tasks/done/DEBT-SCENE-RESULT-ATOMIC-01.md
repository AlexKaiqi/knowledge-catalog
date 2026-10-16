# DEBT-SCENE-RESULT-ATOMIC-01 · 并发场景运行独立发布最新结果

## 债

- 位置：`cli/scene_feature_test.go` 的 `writeSceneResult`（固定 `_results/latest.json.tmp`，当前第 806—810 行）及所有共享节点最新结果写入。
- 信号：2026-10-16 `make test` 在 `.validation/runs/20261016T013030Z-8bd6849a753f/go-test.jsonl` 的 service-initialized、inventory-read-granted、dataset-manage-granted、dataset-defined 四个节点均报 `rename latest.json.tmp ... latest.json: no such file or directory`；同一工作区有另一个场景运行。该次全量记录 `sourceChanged=true`，不能作为稳定源码门禁通过。
- 判定：每次历史记录已经使用独立临时名，但最新副本仍共用固定临时名。两个运行各自写完后，先 rename 的调用移走共同临时路径，后者报 ENOENT；前者还可能发布另一调用的正文。此为结果发布竞争，不是产品协议能力失败。隔离调度实际 writer 的确定性证据在 `.validation/runs/20261016T013600Z-scene-result-atomic/deterministic-red.jsonl`：首调用 nil、正文 second，次调用 ENOENT，history=2。
- 处置：在最新文件同目录按调用分配唯一临时文件，写完并检查 close 后再原子 rename；失败时只回收自己临时文件，保留可读权限、同次独立历史及 latest 非 Oracle 语义。候选补丁 `.validation/runs/20261016T013600Z-scene-result-atomic/candidate.patch` 仅备查，未应用；原文件仍属活跃 INDEX 在途范围，等待该线完成或接管后实施。不能用进程内互斥锁冒充跨进程隔离，也不把记录失败吞掉。
- 触发条件：多个本地场景进程、完整门禁与定向场景并发运行，或任何运行共享同一节点 `_results` 目录时。

## 目标

各次场景结果互不抢占临时文件。并发写入都能完成，latest 始终是一份完整、来自真实调用的报告，独立历史不丢失。

## 依据

- [测试与验证](../../doc/engineering/testing.md)。
- [工程质量闭环](../../doc/engineering/quality.md)。
- 场景执行和证据合同：`.data/scenes/README.md` 执行一节。

## 验收

- 对共享节点确定性安排两次实际 writer 在写完后依次发布，两次均无 ENOENT；第一调用成功时不会取走第二调用尚未发布的正文。
- 同进程和两个独立进程的并发回归均保留完整合法 JSON；每次历史各自独立，不将 latest 用作跨运行 Oracle。
- 写入、close、rename 失败能明确报告并只清理本调用临时文件；既有 latest 和其他调用临时文件不受失败清理影响。
- 原场景结果合同、受影响场景、`make check-docs`、`make quality` 通过，无 skip；门禁源码稳定性单独记录。

## 前置

`cli/scene_feature_test.go` 的活跃 INDEX 修改完成，或依约接管后可安全追加修复。

## 不做

不串行化整套测试来掩盖结果文件竞争，不重置部署或删除共享资源，不改源场景断言。

## 调查证据

2026-10-16 仅在 `.validation` 的 Go overlay 副本插入 rename 前调度钩子，所有文件 I/O 仍调用实际 `writeSceneResult`。原 writer 的一个定向反例失败；同一反例在唯一临时名候选 overlay 下通过（`candidate-green.jsonl`），唯一历史均为两份。源码原文件 SHA-256 调查前后相同，未应用候选补丁；该验证不等于仓内修复或完整并发验收已完成。


## 2026-10-16 实现与验证

实现范围仅为 `writeSceneResult`：latest 使用同目录唯一临时文件，保留 0644，写入并检查 close 后原子发布，失败只回收本调用临时文件。新增独立回归文件覆盖同进程及两个独立进程并发、完整独立历史、可读权限与 rename 失败清理；未修改该既有文件的其它部分。

验证先红后绿：原实现 3 条回归与确定性调度反例失败；修复后结果/实际场景合同 7 pass、故障注入与确定性顺序 5 pass、race 回归 3 pass，均 0 fail / 0 skip。独立快照源码稳定，`make check-docs` 通过；`make quality` 被主线既有 709 行文件门禁阻断，归 `DEBT-DISCOVERY-SEARCH-SIZE-01`，未绕过或降低门禁，保留 doing。命令、源码指纹、红绿与快照证据见 L1 验证索引（`.validation/runs/20261016T015929Z-l1-scene-result-atomic/index.md`）。未安装依赖或提交，未占用共享索引或启动部署。

## 交付

实现范围为唯一临时文件原子发布及并发/故障回归；前述 7 项场景与结果合同、5 项确定性和故障注入、3 项 race 回归已通过且无 skip。补跑稳定快照 `2197705d42c96a5a9451930181ced544a6e6a83967ca08718d9532593b8896d4` 的 `make check-docs`、`make quality` 全绿，完整架构组 657 pass / 0 fail / 0 skip；见 恢复后快照门禁（`.validation/runs/20261016T023225Z-post-recovery-snapshot-gates/index.md`）。源码指纹与每步源码未变化单独记录，原 709 行门禁阻塞在该快照已解除。未提交。
