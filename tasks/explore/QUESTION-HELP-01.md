# QUESTION-HELP-01 · 帮助体系是否可达且示例可执行

## 要回答的问题

从 `kc help` 出发能否走到每个命令；顶层展示名与实际 topic 是否一致；help 里的示例资源在当前服务上
是否可见；约 50 个动词是否都有独立帮助页。

## 已知证据

`kc help branch` exit 1，`kc help writer` exit 0；顶层中文名"分支"对应真名 `writer`。同类不一致还有
"运维"→`operations`、"部署"→`deployment`。`kc help dataset` 混入 `grant` 与 `files` 命令。

`cli/plugin_help_actual_server_test.go` 已有对真实服务的帮助断言，可复用其写法。

## 判定标准

- 顶层主题可达性表：不可达的给出真名，关闭为"命名不一致"；全部可达，关闭为"无问题"。
- 动词级覆盖缺失清单为空，关闭为"覆盖完整"；有缺失，关闭为"确认缺口"并列出。
- 示例资源在当前服务上实测可见，关闭为"示例可执行"；`not visible` 的逐条列出，关闭为"确认缺陷"。
- 两套体系（`identity`/`login`、`operations`/`ops`、`writer`/`branch`）是否应互指，结论须引设计而非命名直觉。

## 前提

无。与 `SYSTEM-GUIDE-EXECUTABLE-01` 分工：该项改系统仓指南正文，本项只做 help 侧可达性与一致性。

## 不涉及

不重写 help 篇幅、不改命令分组结构（分组调整若确有必要，单独立项）、不把系统仓指南搬进 help。

## 结论

待填。
