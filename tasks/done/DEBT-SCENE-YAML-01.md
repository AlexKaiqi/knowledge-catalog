# DEBT-SCENE-YAML-01 · 场景树 YAML 解析器静默截断

认领：With 智能体 · refactor/object-model · 2026-10-15

## 债

- 位置：`.data/scenes/tree.py` 的 `load_yaml` 与 `_strip_comment`。
- 信号：规格条目里的 `#` 之后被当成注释截掉（`schema/x#/fields/y` 只剩 `schema/x`）；含 `http:` 的
  列表项被解析成键值对。2026-10-15 写验收规格时各出现一次，同日改 `repository-attached/_specs/liveaccess.yaml` 时含 `` `identity: shared` `` 的列表项又被当成键值对一次。后两次被规格形状检查拦下，`#` 截断是静默的。
- 判定：同类信号已出现两次；`#` 截断会悄悄改变规格含义，检查却照常通过。
- 处置：升级为自动检查——只有前面是空白或行首的 `#` 才算注释；列表项里出现未加引号的 `key: value`
  而上下文要求字符串时报错。修复后去掉规格里为绕开它加的引号。
- 触发条件：`ACCEPT-HARNESS-01` 之前（它的前置）。

## 目标

规格里的定位符与 URL 原样保留，解析结果与标准 YAML 一致；`.data/scenes/_tests` 里有对应的反例。

## 依据

- `.data/scenes/README.md`。
- [工程质量闭环](../../doc/engineering/quality.md)：三个出口。

## 验收

- 新增反例：含 `#/fields/x` 与 `http://h:1` 的未加引号列表项，解析结果与 PyYAML 一致。
- 场景树三项检查与 `.data/scenes/_tests` 通过。

## 前置

无。

## 不做

不引入第三方 YAML 依赖。

## 交付

- 实现：`.data/scenes/tree.py` 只把行首或空白后的 `#` 当注释；引号只在标量开头起作用；键值只按“冒号加空格”或行尾冒号切分；引号闭合后还跟着文字的标量直接报错（与标准 YAML 一致）。`accept-09` 里唯一一处这种写法改成了不带引号；`accept-12`、`accept-13` 里为绕开旧解析器加的 3 处整行引号已去掉。
- 反例：`.data/scenes/_tests/test_views.py` 新增 3 条（定位符、URL、撇号原样保留；只按冒号加空格切键值；引号后有文字报错）。
- 已验证：用仓库已有的 `gopkg.in/yaml.v3` 解析全部 39 个 `_meta.yaml`、`_views.yaml` 与 `_specs/*.yaml`，结果与场景树解析器逐个一致（本机没有 PyYAML，按“不引入依赖”改用 yaml.v3 作参照；对照程序为一次性脚本，未入库）。`test_views.py` 21 条、场景树三项检查、`make check-docs`、`make quality` 通过。
- 提交：未提交。
