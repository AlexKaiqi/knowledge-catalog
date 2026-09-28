# datasetfs/

Dataset 的本机只读消费视图——把固定 Dataset 消费计划挂载成本机目录（kcfs），零写路径；
`cmd/kcfs` 经 `cli` 消费，协议语义不进本包。

## 边界

拥有：挂载计划（`Plan` / `Target` / `Plan.Validate`）、挂载执行与生命周期（`MountAll` / `Mount` /
`MountHandle` 的 `Unmount` / `Wait`）、只读视图类型（`Directory` / `DirectoryEntry` / `File`）、
平台面（Linux FUSE 实装；非 Linux 编译通过，`MountAll` 返回显式不支持错误；`Options.Debug`）。

不拥有：写路径（Writer 归 `knowledge`）、Dataset 声明与组合语义（`catalog`）、消费授权与范围
（`knowledgeapp`）、协议坐标解释（`kernel`）。挂载语义为固定 commit：目标在计划中固定，运行期不切换版本。

## 最小用法

```go
var plan datasetfs.Plan         // 由 cli 从 Dataset 消费范围填充
targets, err := plan.Validate() // 计划校验与平台无关，可先行
handle, err := datasetfs.MountAll(plan, datasetfs.Options{}) // 仅 Linux；其它平台显式报不支持
err = handle.Unmount()
```

## 依赖方向

可以依赖：仅标准库（当前零 kc 依赖）。
禁止 import：任何 `kc/...` 生产包——消费视图不回调协议环（B2 落 denylist 钉死）。

## 验证

- 单测：`datasetfs/plan_test.go`；
- arch guard：`internal/arch` denylist `datasetfs ↛ 全部 kc 生产包`（B2 落地）；
- coupling unit：`datasetfs`（ratchet 基线内单元）。
