# hook/

**Hook 是出站调用**：在应用动作的 `pre` / `post` 去调用户脚本或 HTTP。不拥有知识，不替代 `allow`，也不是 gate。

配置在服务持久 `stateDir` 下的 `hooks.json`。`pre` 必须 `--run`、超时 fail closed、失败则命令不落盘。`post` 只发指针；HTTP 失败进 `hook-outbox.jsonl`，不撤销已成功命令。`REPLAYED` 不打 hook。读路径不挂 hook。

## 谁被创建

| 对象 | 怎么来 | 之后 |
|---|---|---|
| Binding | `kc operations hook add --on <action> --phase pre\|post` | 匹配的命令触发 |
| Outbox 行 | `post` 投递失败 | 下次 `post` 时 Flush |

## 文件

| 文件 | 负责 |
|---|---|
| `store.go` | `hooks.json` |
| `dispatch.go` | `Pre` / `Post` |
| `exec.go` | `--run` |
| `http.go` | `--url` |
| `outbox.go` | post 失败重试 |

CLI 与 typed HTTP 共用的应用执行器在 `allow` 之后、协议命令前后调用。Writer / Catalog 不 import 本包。Gate 见 `../gate/`。设计见 `docs/reviewed/hooks-and-gates.md`。

`--on` 使用 `store.go` 的可挂接 semantic action 词表，例如 `writer.commit`；不是任意 CLI 字符串。公开分组命令见 [`cli/SURFACE.md`](../cli/SURFACE.md)。
