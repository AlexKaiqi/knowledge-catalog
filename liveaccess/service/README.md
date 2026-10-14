# LiveAccess 逻辑读取

`OpenRequest` 固定声明范围及可信请求依据，通过注入的 StateLookup 返回逻辑 Aspect/Member 值及实际观察 basis。`HydrateRepositoryRead` 复用已读取的 Repository 固定值，不重读整对象或未选单元；没有动态访问时，固定读取仍由 Object Reader 独立提供。

`ReadRepository` 接收中立 `ReadRequest`，来源由传入 Repository 固定。对象根、维护 Address
和 Reference 共用上述路径；动态单位先取完整实际值，再投影 locator，固定声明没有该 leaf
不会提前阻止 Lookup。请求位置冲突、错误来源或非法 Address 在调用来源前拒绝。
可选 `ReadRequest.Hydrator` 复用宿主的既有固定正文缓存；缓存不保存动态采用结果，
当前授权及本次外部观察仍分别执行。
传入已有 Dataset Member 时，Reader 在缓存前核对全部嵌套范围；部分地址沿原精确
作用域读取，不借同 Repo/commit 已热的完整对象或私有地址结果扩大范围。

声明 commit 与来源观察版本分别保留；来源失败不回退旧观察，不把观察写回 Snapshot。
普通 `Read`、`Hydrate`、`ReadAddress`、`ReadReference` 将外部单位失败放在该单位的原位置，
形状为 `ValueFailure{Error: kernel.IngressError}`（JSON 为 `{"error":{"code":"…","message":"…"}}`）。
`ObservedValue.Failures` 给出失败维护位置及原因，与业务值恰有 `error` 字段的情况区分；
只有 Schema 核验及 basis 合法的成功值进入 `Observations`，合法 null 保持 null。
对象、单位或请求本身越出允许范围仍是读取错误；已获准单位的句柄声明解析失败仅影响该值，
不补读未发布材料、不调用未经授权的来源。`HydrateRepositoryValue` 为索引维护采用严格路径，
任何动态失败都返回 error，索引不会采用失败说明或旧值。

Schema origin、遗留 Descriptor、操作及 Bound State 单元合成由 LiveAccess 根入口唯一解释，
服务再采用其结果调用 StateLookup。Dataset 解释先沿 Object 的 Member 取得相同固定范围，
不借来源仓补齐未发布的 Schema，也不让固定声明版本冒充外部观察版本。

`ReadReference(ctx, object.Reference)` 保留 URN 指定的单一来源仓及 Dataset 固定版本。
对象根沿既有 `Hydrate` 执行；非根位置先经 `ReferenceUnitsReader` 取得完整选中单元，
采用相同的 State/Stream 核验，再以根 Object 的 JSON Pointer 选取内容。
固定声明中没有目标 leaf 不会提前阻止 State 观察；没有维护单元时，只有已发布的
Domain Schema 实际声明的 Bound Aspect 才可合成，未知名称明确失败。
选中单元的 Schema／遗留 Descriptor 解释仍由 LiveAccess 同一核心算法采用，
不再次加载整对象和未选兄弟来定位已经选好的实际单元。
Member 从自身实际采用的固定 Schema origin 解释取值，同样核验 Schema 和来源依据，
不以 Member 的固定知识值冒充外部观察。
Member 是否为 Binding 采用 Reader 已固定分类的 `ValueSource`：只对 Binding 读取真实 Schema，
普通 Snapshot Member 不因其 Schema 未发布而变成外部失败。Binding Schema 在作用域外时
保留该单位的失败，不读取私有 Schema、不回退 Member 占位值。

绑定 `evidence.Collector` 的消费请求在采用动态单位后捕获完整原值或失败、固定声明与
原观察依据；selector 排除的单位不调用、不留证。deep locator 的普通结果仅呈现所选
内容，留证保留本次完整单位观察，结合过程账中的 Reference/Locator 复核所选内容。
Capture 失败阻止本次结果交付；没有 collector 的维护调用不生成消费证据。
源错误在呈现及留证前净化当前请求凭证，携带凭证的源结果不能被采用。

静态位置保留所属完整维护单元的 `Digest`；集合没有单一维护摘要。
有动态观察或失败的结果清空此静态维护摘要，以 `Observations` 保存独立的来源依据。
