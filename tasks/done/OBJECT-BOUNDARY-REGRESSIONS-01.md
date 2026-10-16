# OBJECT-BOUNDARY-REGRESSIONS-01 · 重构审查的针对性回归（2026-10-13）

- [ ] 本轮仅补用例：用户明确要求等待正在进行的重构完成后再进行下一轮重构；按目标合同新增并执行针对性回归，保留当前缺口的失败证据，不修改实现、不以现状缺陷作为通过期望。
- Goal / owner：表示一致性、真实只读端口与请求责任沿 `object-refactor-guideline.md` / `knowledge.md`；观察核验及访问证据沿 `resource-access.md`；请求来源与后台来源隔离沿 `index-control.md`。用例最低责任层和证据语义沿 `test-catalog.md`，迁移分母沿 `object-refactor-plan.md`；已核对 docs/graph 所有权及直接关系。
- Non-Goals：不修改生产 Go、公开形状、CLI/HTTP、权限、来源/Schema 声明或后台装配，不接入插件，不干预其它任务的在途实现，不新增依赖、启动服务或提交。不为保持绿色删除/放宽现有断言，也不使用 skip 或构建标签隐藏预期失败。
- 不变量 / 接口：I-01 / V-01、W-01 / W-02、AUTH-01 / AUTH-03、API-01、OI-01～05、PC-01；使用公开 Writer、Reader、StateLookup、实际访问入口和 ProjectionNoticeExecutor 的既有合同及现有内存夹具，不另造插件或任务 DTO。
- 选定：Writer 在无 Schema 下也拒绝最终表示冲突且不推进 Ref；Reader 的具体返回值逐项不携带创建 Ref、Merge 或 Archive 能力；直接 access 拒绝非法观察依据/违反实际采用 Schema 的值，并保留合法空值及实际访问证据；两个请求身份不能替换装配期后台来源。每项含能到达目标检查的合法前态及控制组。
- 否决：把缺陷复现成功写成目标合同通过、只断言任意错误/函数名/目录、为测试方便发明未选定授权上下文或 API、把普通 audit 当实际访问证据，以及把包迁移当行为修复。
- 审查项保留：Repo READ/COMMIT 的任务规则仍在 Invocation、Object 根仍拼装访问 endpoint，判定为职责收口事项；本轮不编写绑定未选定字段或静态函数位置的伪行为测试，触发为当前重构完成后的下一轮职责复核。无 Schema 表示冲突和真实只读端口已由无服务临时探针确认，未判定是否由本次重构引入。
- 验证出口：新增测试必须编译并单独执行，明确目标合同的红/绿结果及失败原因；运行 `make check-docs`、`make quality` 和增量检查，仅报告实际范围。因用户本轮只授权用例，未修复的目标合同报红时保持本条未勾选；下一轮修复后复跑所需合同至绿，不使用本轮局部结果宣称当前重构或部署已验收。
- 用例落点：`object/writer/representation_consistency_test.go` 覆盖同批两顺序及既有 Record/Member 的冲突、Ref/原对象/回执不变和合法 REMOVE→PUT 转换；`object/reader/readonly_view_test.go` 覆盖 tree/native × Require/Wrap 的具体能力及固定读值/坐标；`invocation/direct_access_contract_test.go` 经实际应用入口验证非法观察、固定 Schema、合法值/null、成功/源失败的耐久 Access 证据；`discovery/service/notice_background_source_test.go` 通过真实 Source 桥接核验两个 caller 以及拒绝路径之后的后台实际来源、主体、Trace 和请求关联。
- 独立复跑：四包新增用例均编译成功，合法转换及合法值/null 控制通过；冲突表示/只读端口/直接 access 核验与证据/后台来源隔离仍真实报红，组合命令退出 1。最终 JSON 事件保存在 `.validation/object-boundary-regressions-20261013/targeted.jsonl`，各项首轮输出分别是同目录 `object.log` / `access.log` / `notice.log`；没有 skip、构建标签或现状迎合断言。未改生产实现，未执行部署验收或整套契约。
- 质量出口：已有 `TestProjectionNoticeExecutorConfiguresRuntimeBeforeNotice` 锁定请求替换来源，与本轮目标隔离断言冲突；本轮保留旧测试，下一轮改接 notice 时须按 owner 合同复核其 oracle。服务规则与运行 endpoint 的归属继续作为职责审查项；不要求当前在途重构同时完成插件接入。新增文件的空白检查通过；门禁结果随本轮最终输出登记，不把行为红证据记作验收通过。
- 最终门禁：`make check-docs` 通过（33 文档 / 168 关系），`make quality` 退出 0，格式/tidy/vet/staticcheck、生产复杂度/体积/克隆和耦合棘轮通过，日志 `.validation/object-boundary-regressions-20261013/quality.log`；仅工程闸绿，目标行为仍红。测试自检允许既有内部证据包装而只约束公开输出，源失败钉原错误分类且不能制造成功观察；notice 拒绝钉实际到达控制路径及错误分类，不约定错误对象地址。下一轮实现使用本轮正式回归，不将临时“复现缺陷成功”的探针作为目标通过条件。

## 关闭

2026-10-15 拆分 `TASK.md` 时关闭，未按原条目验收。早于 2026-10-14 对象模型重构的认领，相关代码与设计已被后续重构改写（见 `0759640` 之后的提交），原范围按旧结构写成，不再继续。整体验收并入 `FINAL-ACCEPTANCE-01`，插件相关用例由验收规格 accept-01～13 取代。
