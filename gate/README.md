# gate/

**Gate 是 `merge` / `promote` 的证据清单**：跃迁前查钉死的 Preview / Generation 上有没有必过的绿记录。不拨用户系统、不改知识、不是 hook。

配置在工作区 `.kc/gates.json`，不是成员仓对象。无清单时保持今天的单门雏形（`merge` 仍带一份 PASSED validation）。

## 谁被创建

| 对象 | 怎么来 | 之后 |
|---|---|---|
| Rule | `kc gate-add --on merge\|promote --require …` | `merge` / `promote` 读清单 |
| Evidence | `kc validate`（suite `structure`）或 `record-validation --suite` | 必须绑同一 basis |

`validate` 匹配 `structure`；`suite:name` 匹配该 suite。调用方不能靠少传 `--validation` 跳过清单。`Rollback` 不跑 promote gate。

## 文件

| 文件 | 负责 |
|---|---|
| `check.go` | `Check` / `--require` 解析 |
| `store.go` | `.kc/gates.json` |

ControlPlane `Merge` 与 Catalog `Promote` 调用 `Check`。Hook 出站见 `../hook/`。设计见 `docs/GATES.md`。
