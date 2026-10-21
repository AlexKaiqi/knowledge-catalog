# ARCH-DATASET-PIN-01 · Dataset 读取依据转换归一

## 目标

`home.DatasetPin`、`internal/testkit.DatasetPin` 与插件测试中的 `pluginDatasetPin`
复用唯一的生产转换实现，来源版本、文件范围和既有解释依赖不因调用路径而丢失。
转换不依赖 Home、插件运行时或测试设施，也不增加 Reader 对 Dataset 服务的依赖。

## 依据

- [Dataset](../../doc/design/dataset.md)、[对象](../../doc/design/object.md)、[服务](../../doc/design/service.md)。
- [架构不变量](../../doc/engineering/architecture-invariants.md)、[工程质量闭环](../../doc/engineering/quality.md)。
- 实施时核对 `snapshot/dataset`、`object/reader`、`discovery/datasets` 的 README、公开类型与分层守卫。

## 范围

建立拟名 `internal/datasetbridge/` 的上层私有纯转换，输入输出沿现有合同；
它只依赖 Snapshot Dataset 与 Object Reader 的所需公开类型，不取得服务句柄、访问来源或解析 HEAD。
根 Home 仍装配发布准备、冲突和依赖回调；桥接包不接管这些业务。
既有公开 `home.DatasetPin` 若有兼容需要只委派唯一实现；测试辅助不保留字段复制算法。

当前 object 层的依赖 allowlist 不含 dataset 层，不能直接把转换塞入 `object/reader` 并放宽整层规则。
桥接包显式按上层归类，同时限制其可依赖范围，保留 `discovery/datasets` 不依赖 Reader 实现的守卫。

## 验收

- 独立手写输入与预期覆盖身份、修订、来源提交、prefix/file 选择、空范围和已有解释依赖；
  每个当前字段均有归属，非空依赖向量在生产、共享夹具和插件测试入口得到相同正确结果。
- 来源或依赖 HEAD 随后推进不改变已转换的坐标；依赖仓不因转换成为选入成员，
  空选入范围不被扩大为整仓。原固定读取与范围隔离回归继续通过。
- 只存在一份字段映射；测试不能调用转换生成自己的预期。重复信号升级为有正反例的
  所有权/依赖守卫，新同类复制不能绕过检查。
- 桥接包不直接或间接依赖 `home`、`invocation`、具体插件或 `internal/testkit`；
  Reader、Snapshot 及 Dataset 发布执行器原有禁止依赖不被削弱。
- 执行 `ARCH-LAYOUT-01` 的共同实施与验收约定；覆盖 Reader、Dataset 发布准备、插件恢复与共享夹具的实际消费者。

## 前置

本项后置，不再按“当前字段暂时稳定即可先迁”安排，但仍不依赖 Home 拆分。

- `OBJECT-VERSIONING-01` 的“固定读取依据交付”：完整候选来源、选择范围、来源绑定与解释依赖
  已有统一实现，固定回读和授权回归通过；不能先按旧 pin 复制一套桥接形状。
- `INDEX-PHYSICAL-01` 的“索引接入与接口稳定交付”：索引已经消费上述依据，
  发布准备和回读接缝有联合回归，转换的真实输入、输出和消费者可据此固定。
- 接入 `INDEX-SEARCH-SPACE-01`、Dataset 版本任务涉及的已交付批次后，再记录转换迁移清单与源码候选。
  不要求相关任务整项归档，但前两项交付点是必需条件，不只是同文件并发提醒。

若行为重构已改变三份旧转换的位置或消除其中一份，按最终消费者归一，不恢复旧字段或重复实现。
Object 与 Index 不以此目录任务为前置；因行为正确性必须同步的映射由行为任务同批处理，
本任务只负责稳定后的归位及残留收口。

## 不做

不新增完整读取依据协议，不修复普通新请求与精确回读的语义差异；
这些仍归 `OBJECT-VERSIONING-01` 及其债。不改发布版本分配、授权、索引准备或依赖解析算法。
