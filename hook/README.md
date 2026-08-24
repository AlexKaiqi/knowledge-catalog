# hook/

**Hook 是出站调用**：在现有 `kc` 动词的 `pre` / `post` 去调用户脚本或 HTTP。不拥有知识，不替代 `allow`，也不是 gate。

配置在 `.kc/hooks.json`。`pre` 必须 `--run`、超时 fail closed、失败则命令不落盘。`post` 只发指针；HTTP 失败进 `.kc/hook-outbox.jsonl`，不撤销已成功命令。`REPLAYED` 不打 hook。读路径不挂 hook。

## 谁被创建

| 对象 | 怎么来 | 之后 |
|---|---|---|
| Binding | `kc hook-add --on <cmd> --phase pre\|post` | 匹配的命令触发 |
| Outbox 行 | `post` 投递失败 | 下次 `post` 时 Flush |

## 文件

| 文件 | 负责 |
|---|---|
| `store.go` | `.kc/hooks.json` |
| `dispatch.go` | `Pre` / `Post` |
| `exec.go` | `--run` |
| `http.go` | `--url` |
| `outbox.go` | post 失败重试 |

CLI facade 在 `allow` 之后、协议命令前后调用。Writer / Catalog 不 import 本包。Gate 见 `../gate/`。设计见 `docs/HOOKS.md`。
