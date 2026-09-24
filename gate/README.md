# gate/

**Gate 是 `merge` 的证据清单**：跃迁前查钉死的 Preview 上有没有必过的绿记录。不拨用户系统、不改知识、不是 hook。

配置在服务持久 `stateDir` 下的 `gates.json`，不是成员仓对象。没有匹配清单时走兼容路径（`merge` 仍带一份 PASSED validation）。`--on` 只接受 `merge`。

## 谁被创建

| 对象 | 怎么来 | 之后 |
|---|---|---|
| Rule | `kc operations gate add --on merge --require …` | `merge` 读清单 |
| Evidence | `kc governance preview validate`（suite `structure`）或 `kc governance validation record --suite` | 必须绑同一 `previewId` |

`validate` 匹配 `structure`；`suite:name` 匹配该 suite。调用方不能靠少传 `--validation` 跳过清单。

## 文件

| 文件 | 负责 |
|---|---|
| `check.go` | `Check` / `--require` 解析 |
| `store.go` | `gates.json` |

ControlPlane `Merge` 调用 `Check`。Hook 出站见 `../hook/`。设计见 `docs/reviewed/hooks-and-gates.md`。

## 公开操作

最小公开操作路径：

```bash
kc operations gate add --on merge --repo kr://acme/public/core \
  --require validate,suite:approval:steward
kc governance preview validate --preview <preview-id>
kc governance validation record --preview <preview-id> \
  --suite approval:steward --outcome PASSED
kc governance proposal merge --proposal <proposal-id> --preview <preview-id>
```

`require` 是一条逗号分隔的完整清单。存在匹配 Gate 时，`merge` 从已保存的
Proposal 推导授权所需的目标 Repository/Ref，并检查该 Preview 上所有已保存的
证据；调用方不重复传 Repo，也不传 validation ID 数组。成功回执返回
`repository`、`targetRef` 和 `gate {status,basis,required}`，它就是公开证据，
无需读取 `control.json` / `gates.json`。没有匹配 Gate 时，兼容路径仍要求一个
PASSED 的 `--validation <id>`。
