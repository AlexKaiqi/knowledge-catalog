# OBJECT-GUIDELINE-REWRITE-01 · Object 边界与合同重写（2026-10-13）

- [x] 本轮边界纠正完成：明确 Object 写入与 Snapshot 字面写入各自输入、语义和保证，保留六项契约及五项承诺，源码不超过 1500 字符；只改本篇与任务记录，代码实施仍停止。
- Goal / owner：沿 `object-refactor-guideline.md` 的基础模块边界，保留 `knowledge.md` 的身份/维护/Schema 与 `service.md` / `permissions.md` 的任务和当前授权约束；已核对 docs/graph 及现有 Object/Reader/Writer 公开合同。
- Non-Goals / 不变量：不新增“业务 Object／对象知识表示”两套主体，不改 Go、测试、CLI、协议、插件方案或其他任务的在途实现；保持 I-01/V-01、W-01/W-02、API-01、AUTH-01～03、CA-01、OI-01～05、PC-01，不提交。
- 选定 / 否决：用可操作的对象定义、要求→承诺、读写过程和责任表指导重构；否决概念铺陈、预设空目录、重复专题及把测试修补或包迁移写成整体目标。协议形状仍沿公开类型/包 README/Conformance，插件方案仍归各专题。
- 验证：独立复核内容与直接关系，重读改动 owner 和文档图，运行 `make check-docs`、`make quality` 与增量检查；不以文档闸宣称当前红回归或重构已验收。
- 结果与证据：正文从概念/目录铺陈改为对象定义、要求→承诺、描述/写入/读取/当前态流程、责任边界及重构要求；删除并列的“业务 Object／对象知识表示”表、预设目录树、历史图和迁移进度。独立复核纠正了显式历史 Schema 引用与 COMMIT/PROPOSAL 的保证范围；保留现有合同与五段文档格式。`make check-docs`（33 文档 / 168 关系）、`make quality`、增量空白检查通过，质量日志 `.validation/object-guideline-rewrite-20261013/quality.log`。只修改本篇和任务记录，无 Go/测试/图关系/插件方案/部署/提交变化；原红回归继续待实现。
- 本次字数口径：全文 Unicode 字符数包含标题、Markdown、链接及空白，须不超过 1500；保持五段文档合同。压缩后的源码重新复核与执行门禁，前次绿色不作为本次结果。
- 压缩结果：全文 1499 字符、55 行，独立复核确认关键合同保留，明确 Object 只解释固定声明；字数断言、`make check-docs`、`make quality` 和增量检查通过。质量日志 `.validation/object-guideline-rewrite-20261013/condensed-quality.log`。未改代码、测试或文档关系，原红回归与代码实施状态不变。
- 术语纠正：删除 Object 定义中的笼统“来源与能力声明”和表对象示例的“取值声明”；来源依据与特定插件接入分别解释。核对现有 provenance、Schema 的来源入口及既有实现 Member 后，按 LiveAccess 自身合同说明 Schema/Binding 配置，未增加通用 Object 属性。全文 1497 字符，文档/质量/空白检查通过，日志同目录 `access-wording-quality.log`；仅本篇与任务记录变化。
- 本轮结果：六项契约单独定义，五项读写承诺单独说明，保留必要的读写流程与模块边界；全文含 Markdown、链接、空白为 1493 字符。独立复核、字数断言、`make check-docs`（33 文档 / 168 关系）、`make quality`（退出 0）与增量空白检查通过，日志同目录 `contracts-first-quality.log`；未改代码。文档声明的是重构目标，既有行为红回归仍待后续兑现。
- 分层纠正：对象维护的身份、范围、表示与 Schema 保证归 Object；路径/字节及版本提交归 Snapshot，对象变更与文件树变更分别定义。将“唯一写面”限定为“唯一对象写面”，不否定字面写入；保留现有合同，本轮不新增通用整对象覆盖 API。全文 1487 字符，独立复核、文档图重读、字数断言、`make check-docs`、`make quality`（退出 0）及增量空白检查通过，日志同目录 `layer-boundary-quality.log`；未改 Go、测试、图关系或其它专题，原红回归仍待实施。
- 定位审查债：`object/schema.go` 的 `parseSchemaFields` 接受含点字段名，`ValidateSchemaInstance` 按顶层字面键取值；`discovery/fields.go` 的 `lookupPath` 按点拆成嵌套路径，数组只作末值展开。判定为跨模块字段含义不一致，不能复用为统一嵌套定位合同；本轮仅静态核验并登记，未新增 API 或执行行为验收。触发为下一轮字段路径设计/嵌套访问接入：沿 `knowledge.md` 与声明式索引 owner 明确唯一解释，补跨模块回归后再实现，不以当前三段 Address 限制未来内容读取深度。登记的文档及工程门禁通过，质量日志同目录 `address-review-quality.log`。
