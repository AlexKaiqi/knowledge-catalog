# declarativeindex/control

本包维护声明式索引插件的有效服务需求：实例承担的 Repository HEAD，以及当前未退役
Dataset 已接受的固定来源版本。查询实现和引擎归 `../projection/`；实例装配、资源所有权
和来源配置归 `../plugin/`。Object 的正文缓存与独立 Snapshot 消费者继续归
`object/maintenance`，控制器不再创建或注册它们。

## 需求与维护

`NewController(index, store, lookup)` 只构造控制器和持久工作账，不启动后台任务。
`MaintenanceIndex` 是控制器使用的窄口：Snapshot 准备、只读描述、State 声明检查、
实际 State 投影就绪检查及观察刷新；不包含查询或物理引擎定位。
查询与控制共同使用 `kc/discovery/projection` 的公开 DTO。

`SetInventory` 提供本实例当前承担的仓；未设置或显式空清单均不从历史工作账补仓。
`SetServingBases` 逐仓提供已接受的固定来源版本。回调只供应需求，不执行构建，也不把
未接受的候选当成长期需求。控制器复制、去重完整清单后一次替换；任一需求读取失败
保留上一份已核验清单，并返回原错误。首次需求读取失败不能激活后台任务。

`Desire` 和 `Notify` 只登记耐久提示并唤醒后台，不在 Writer 回执路径读取对象、取值
或访问引擎。工作账不是维护资格：`CatchUp` 先刷新需求，再核对当前 HEAD，处理仍适用的
Snapshot 工作，并逐一恢复固定版本与 State。HEAD 不可读时仍尝试独立的固定版本；
一个仓或版本失败仍继续其他项，返回的聚合错误保留原错误供 `errors.Is` 核验。
`Reconcile` 只核对需求和 HEAD、登记必要工作，不构建投影。

全实例 `CatchUp` 在需求与投影工作后还调用实现提供的可选 `RecoverResources(ctx)`；
READY 工作账不会省略资源恢复。错误加入本轮维护诊断，不把已 READY 的对象目标改写成失败。
仓范围 CatchUp 不运行全实例回收；只读 `Reconcile` 同样不运行。取消继续传播到物理恢复请求。

`PrepareServingBasis(ctx, repo, commit)` 在明确固定版本上准备 Snapshot 投影，供上层
Dataset 发布前调用。它不推进 HEAD，不改变登记发布记录，也不借空身份访问动态来源。
`Targets` 返回 HEAD 工作进度；进度完成不证明物理投影仍存在，周期维护仍核对实际状态。

## State 与通知

State 按有效 HEAD 和固定 Dataset 声明分别维护。`StateView` 只表示本进程保存的观察
版本；`StateProjectionReady` 只读核对本地观察和实际提供方依据，并可恢复持久化且与提供方精确匹配的不可变值。冷启动恢复不重新外呼；媒体缺失时重新准备；
真实读取故障保留原错误，不伪装成空结果。后台变化通知作用于各有效声明版本，不把旧
Dataset 的 Binding 换成当前 HEAD。失去需求的旧通知不会再触发外部取值。
HEAD 依据暂时不可读时保留通知；固定版本观察成功不代表整条通知完成，HEAD 恢复后无需重发。

同一通知键使用持久队列代次区分每次入队。旧任务只完成自己取得的代次；其外呼期间到达的
新通知仍保留，即使两次都没有 `SourceRevision` 提示。取值和索引发布仍由索引实现完成，
通知不携带正文，不直接写入仓。

`SetObservationSource` 装配独立的后台来源与身份。后台只借执行上下文的取消和截止信号，
不继承请求中的身份或凭据。`ApplyNotice` 是仅针对当前显式通知的前台入口：有后台来源时
仍使用它的独立身份；未配置时仅为本次借用 request source，不覆盖配置或处理其他排队通知。
显式 ref 保持该入口的既有含义；它不成为后台固定 Dataset 的新依据。

## 生命周期与验证

`Start(ctx) error` 先核验完整初始需求，再运行启动、唤醒及周期维护。构造或一次性打开
客户端不隐式启动；提供方就绪是异步结果。`Close` 取消并等待后台工作，不释放查询引擎；
插件根在已接受请求排空后关闭资源账中的实际 Index 一次。

本包控制用例与 `../plugin/` 的真实 Entry 用例共同验证固定版本恢复、HEAD 故障隔离、
退役工作停止、同仓版本独立失败、旧 Binding 的 State 恢复、实际媒体丢失和通知竞态。
实际提供方及部署验收按各自运行合同另行执行。
