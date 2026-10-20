# SYSTEM-SEARCH-ENTRY-01 · 系统仓内容对 agent 的可检索入口

## 目标

agent 读系统仓时，除了从导读逐级点下去，还有按内容找到目标说明的路径；找"成员该不该做"不会因为不知道它在 `concepts/member/` 而无从下手。

当前状态：系统仓已声明索引（`index/system/explanation-body.yaml`，索引 `schema/system/explanation/v1#/fields/body`，`build.kind: text`），但检索只在被选进 Dataset 时生效，而实际部署与场景里没有任何 Dataset 把系统仓选为来源，所以这份声明目前不被消费。agent 的默认路径只有导读。

## 依据

- [发现](../../doc/design/discovery.md)：检索在 Dataset 内按成员构建；声明存在不等于已就绪或被消费。
- [Dataset](../../doc/design/dataset.md)：Dataset 显式选择来源与版本，组合不要求先解释所有 Schema。
- 系统仓概念 [system-repository](../../systemrepo/content/concepts/system-repository/definition.md)：业务 Dataset 显式选择采用其中的文件。

## 验收

先选定形态再实施，二者取一并写明理由：

- 方案一：提供由系统仓内容构成的只读 Dataset（固定 revision），agent 通过既有 `kc` 检索原语在系统仓范围内查找说明；不新增命令。
- 方案二：保持不检索，改为在导读里给出足够细的分类导航与关键词索引，使"按意图找说明"有确定路径。

落地后须满足：

- 真实可查：以给定意图词（如"成员""切面""复用"）能找到对应说明，且结果带对象坐标与固定 revision。
- 未就绪、未授权、无实例三种情况下明确失败，不冒充零命中；不因未配置索引而静默返回空。
- `go test ./systemrepo/...`、`make check-docs`、`make quality` 全绿；受影响检索回归通过，无 skip。

## 前置

- `SYSTEM-GUIDE-EXECUTABLE-01`、`SYSTEM-MODEL-GUIDE-02` 先落定，避免索引建立的分类随后被重排。

## 不做

不为系统仓新建独立检索服务或专用命令；不让系统仓内容进入业务 Dataset 的默认搜索范围；不改变"检索在 Dataset 内"的既有合同。
