# QUESTION-PROGRAM-01 · 退出码与输出流能否被脚本稳定消费

## 要回答的问题

错误是否稳定写 stderr、退出码能否区分用法/业务/权限/网络四类、错误格式是否单一、版本号能否被脚本取值。

## 已知证据

`kc dataset show __x__`：stdout 98 字节、stderr 0 字节、exit 1，错误 JSON 走 stdout。
`kc --version` 输出完整 banner 无版本号；`kc version` 报 `unknown command version`。
`kc help branch` exit 1，真名 `writer` 可达。

任务书称"网络超时、服务 EOF、revision 越界全部归为 USAGE_INVALID"，但 `dataset show __x__`
实测为 `DATASET_INVALID`，该判断需重跑确认。

## 判定标准

- 五类错误（用法、业务、权限、网络、超时）各一例，退出码与输出流成矩阵；同类不同码或同码不同因，
  均需逐条说明。
- 错误写 stdout 出现一次即关闭为"确认缺陷"（脚本无法用 `2>` 分离）。
- 版本探测有无可用入口，结论二选一：已有入口可取值，或确认缺失并说明建议形态。
- `--limit 0`、revision 越界、pointer 越界、互斥参数的文案逐条记录，指出哪些同码不同因。

## 前提

无。与 `ARCH-ERROR-CODES-01` 不改同一批文件：本项只做消费侧观测，不改错误常量定义。

## 不涉及

不重排错误码体系、不改 HTTP 状态映射、不改错误信封结构、不新增子命令（版本探测若确认缺失，
单独立项后再实现）。

## 结论

待填。
