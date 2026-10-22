# QUESTION-AUTH-01 · 权限门禁在各入口是否一致

## 要回答的问题

同一身份在 `show`、`dataset show`、`schema`、`files`、`search`、`read`、`relations`、`traverse`、
`registry audit`、`grant list` 各入口看到的可见范围是否一致；`KC_AS` 在 `local` 之外的认证模式下
能否覆盖身份。

## 已知证据

[permissions.md:43](../../doc/design/permissions.md) 规定"没有消费权的 Dataset 不出现在库存列表里"。

实测 `KC_AS=nobody kc show` 返回 datasets `autumn-launch-kit`、`qinghe-analysis`、`store-oncall`，
不含 `qqbrowser`，库存过滤成立。但 `KC_AS=nobody kc dataset show qqbrowser` 返回 `itemCount: 39`
与全部 items。两处对同一身份给出不同可见范围。

## 判定标准

- 若 `dataset show` 返回的字段全属"定义"且 `permissions.md` 允许仅凭 id 读定义，则两处不一致属设计
  分工，问题关闭为"无缺陷"，登记文档缺口即可。
- 若返回内容含成员 commit、路径等"内容"字段，则与"发现与读取分开"冲突，关闭为"确认缺陷"，另开任务。
- 若 `KC_AS` 能在非 `local` 模式覆盖身份，属身份伪造，按 P0 单独立项。

上述任一结论都必须带逐入口矩阵作为证据。

## 前提

无。

## 不涉及

不修改 grant 规则、不新建持久授权、不把探测用的降权身份写入配置、不改身份绑定存储格式。

## 结论

待填。
