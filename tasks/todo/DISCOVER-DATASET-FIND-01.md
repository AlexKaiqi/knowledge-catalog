# DISCOVER-DATASET-FIND-01 · 找 Dataset 这一步可积累

## 目标

“先找 Dataset”这一步和 Dataset 内搜索一样留下可归因的使用记录：库存列举的条件和意图、列出了哪些
Dataset、调用者选了哪个、选完是否在里面得到采纳、是否换到别的 Dataset。按 Dataset 汇总后公开给
Dataset 的维护者和库存的维护者：被列出、被选中、在其中得到采纳、被放弃的次数，以及“列举后没有选中
任何 Dataset”的意图引用。维护者据此改 Dataset 的名称、用途说明和元数据，或新建 Dataset。

库存列举本身的匹配方式不变，仍按消费权限、名称和用途做简单匹配。

## 依据

- [发现](../../doc/design/discovery.md)：找到 Dataset、使用记录、U10。
- [Dataset](../../doc/design/dataset.md)：用途说明与元数据由维护者维护。

## 验收

- `discovery.md` 在 U10 上扩展或新增用例，场景树规格全绿。
- 列举、选中、Dataset 内采纳能按会话串联；没有消费权的 Dataset 不出现在列举和汇总里。
- 汇总不扩大谁能看到谁的访问，样本只给引用。
- `make check-docs`、`make quality`、`make test` 通过。

## 前置

`DISCOVER-INTENT-01`。

## 不做

不按积累的记录改变库存的列举排序；不做 Dataset 的语义匹配或推荐；不跨 Dataset 合并搜索。
