# DEBT-EVIDENCE-CITED-RUN-MISSING-01 · 被跟踪文档引用的证据 run 已永久缺失

## 交付（2026-10-21）：增量守卫已落地

存量处置未动（`tasks/done/` 是历史不改写，`.validation/` 从不入库，缺失不可恢复），但把“增量不再
丢失”从保留策略延伸成一道自动检查：

- 新增 `scripts/check-docs/evidence_citation.go`，接入 `checkDocumentation`：`doc/` 下任何设计文档
  出现 `\d{8}T\d{6}Z-[0-9a-f]{12}` 形状的 run id 即失败，错误信息点明“证据从不入库，请陈述结论”。
  设计文档只写结论与承载它的合同；run id 留在任务记录里，那里有日期与范围，读得出这是历史注记。
- 任务文件不扫描，这是有意的：`tasks/done/` 不再改写，把存量 90 处改成无 id 叙述只会制造假精确。
- 回归 `scripts/check-docs/evidence_citation_test.go` 四条：设计文档引用被拒、任务记录放行（含补齐
  `tasks/` 目录结构的夹具）、提交摘要与日期等形似 token 不误伤、真实 `doc/` 全量通过。

本轮用与研究相同的口径复核了存量规模：扫描 58 个被跟踪文件中形似 run id 的字符串，得到 297 个不同
id，其中 90 个在 `.validation/runs/` 下不存在——与本债原先的 292/90 一致（差异来自本轮新增任务文件）。
缺失分布集中：`snapshot/console/webui/CONSOLE-VALIDATION.md` 15 处、`tasks/done/DEBT-SCENE-META-TESTNAMES-01.md`
13 处、`tasks/done/OBJECT-REFACTOR-EXECUTION-01.md` 11 处、`tasks/done/CLEANUP-03.md` 9 处。
其中唯一的维护文档 `CONSOLE-VALIDATION.md` 已在开头声明“运行 ID 保留其当时范围，不能作为当前验收”，
这些 id 是它刻意保留的历史坐标，不该按缺陷改掉。

`make validation-prune` 本轮预演：kept 258、would delete 7（13M）、112 个无 manifest 目录 2426M，
被引用 run 均标注 `cited by a tracked file`，保留侧行为与债的判定一致。

## 信号

`make validation-prune` 的引用保护扫描当前工作区中被跟踪与新加入文件里的 run id，得到 292 个
不同 id；其中 90 个在 `.validation/runs/` 下不存在。也就是说，`tasks/` 里当下仍有 90 处
“证据见 `<run-id>`”指向已无法打开的结果。历史更大范围的丢失无法从当前工作区估算，
因为 `.validation/` 被忽略、从未入库，删除即无副本。

## 判定

这是证据保留策略缺位造成的既成损失，不是本轮改动引入。本轮加入的保留策略只能防止增量再丢失，
不能恢复已删内容；同时它把这个事实变成可观测的：改动前没有任何入口会报告“被引用的 run 不见了”。

根因是两层缺失叠加：其一，`.validation/runs/` 无任何保留上限与清理入口，历史累积到 3.6GB 后
只能靠人工整目录删除，删除不区分是否被引用；其二，`.validation/` 被 gitignore，证据本身不可恢复，
所以引用一旦失效就只能改文或删引用，没有重新取证的路径。

## 处置

- 增量侧已具备保护：`scripts/validation.py prune` 对 `git grep --untracked` 命中的 run 一律保留，
  即使超出 scope 与全局两个窗口；删除只作用于带 `manifest.json` 的 runner 目录。
- 存量 90 处失效引用尚未处置。可选方向：逐条核对引用点，把“证据见 run”改为陈述结论并把
  run id 降为历史注记；或在确认结论已由当前门禁覆盖后删除该引用。不在恢复不了的前提下
  伪造替代证据。

## 触发条件

下次有人需要按 `tasks/` 里的 run id 复核某个结论时触发；或引用缺失数继续增长时触发。
复核入口：`make validation-prune`（预演不会删除，被引用 run 会在 keep 行标注 `cited by a tracked file`）。

## 关闭

2026-10-21 按“不处置存量、只保增量”关闭，不再留在 `todo/`。

增量侧已交付并可执行：`scripts/check-docs/evidence_citation.go` 随 `make check-docs` 接管，
`doc/` 下任何设计文档再出现 run id 形状即失败，回归四条全绿。存量 90 处不做改写有三个理由：
`.validation/` 被 gitignore、被引用 run 不可恢复，改写只会把“当时确实跑过”换成假精确；
`tasks/done/` 是历史不改写；唯一在维护范围内的 `snapshot/console/webui/CONSOLE-VALIDATION.md`
已在开头声明运行 ID 保留当时范围、不作为当前验收，这些 id 是它刻意保留的坐标。

`make validation-prune` 的引用保护与债的判定一致：被引用 run 一律在 keep 行标注
`cited by a tracked file`，预演不删除。

剩余风险与重新登录条件：若此后有结论必须按已删除的 run 复核，该结论即失去证据，应按当前门禁
重新取证而不是引用旧 id；若缺失引用继续增长（说明有人绕过守卫改历史文件），重新登记一条债。
