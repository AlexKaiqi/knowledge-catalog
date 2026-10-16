# OBJECT-PLUGIN-DESIGN-01 · 插件扩展目标、CLI 与重构路线（2026-10-13）

- [x] 本轮完成文档：将 publish/discover/access 的插件扩展设计合入已有目标架构与 R0～R3 计划，明确核心、宿主、声明式索引、LiveAccess 和 CLI 的边界及迁移出口；未实施任何重构阶段。
- Goal / owner：目标与扩展生命周期归 `doc/design/object.md`，路线归 `object-refactor-plan.md`，能力说明及 CLI 载体分别归 `object-system.md` / `terminology.md` / `cli.md`；核心及组织边界沿 core-architecture / capability-organization，所有权与直接关系仅由 docs/graph 登记。
- Non-Goals：仅修改文档与图登记，不实现插件宿主、管理命令或代码迁移，不改变现有协议、Action、命令结果与授权，不发布 system 内容、不运行服务或部署验收、不安装依赖、不提交；保留其它会话的在途改动。
- 不变量：保持 A-01 / L-01、API-01、I-01 / V-01、W-01 / W-02、R-01 / R-02、AUTH-01 / AUTH-03、CA-01、PC-01 与 OI-01～05；安装不发权，范围不等于代码沙箱，基本发布不等待插件/索引/来源在线。
- 选定：系统拥有不可跳过的任务边界，Host 管包/安装/请求视图及可撤销资源，插件注册类型化服务和参与点；默认绑定保留既有能力，Repo/Dataset 显式选择且冲突拒绝。保持 R0 验收、R1 必要行为修正、R2 统一结构切换、R3 完整验收；R2 内按受限基座→Host→标准插件→动态桥接→CLI/装配收尾组织依赖。
- 否决：任意事件或 middleware 重排授权/依据/交付，metadata 执行业务代码、组合自动继承、每插件生成命令树、安装即发权、普通查询启动重建、写完设计即称实现通过。
- 接口与质量出口：设计引用既有 Go 类型/README/Conformance，不复制新字段、错误码或状态机。新增管理能力实施前独立选定 typed API、当前权限、审计与恢复；默认 access 的 basis/value 校验、专用观察证据、外呼目标/redirect 和既有 invoke 说明差距按指南登记位置/信号/判定/处置/触发，未在本轮修复。
- 验证：变更 owner 与图登记已重读，`make check-docs` 通过（32 文档 / 149 关系），`make quality` 退出 0，格式、tidy、vet、staticcheck、复杂度、体积、克隆及耦合棘轮通过；增量与空白检查通过。只证明设计及工程门禁，不外推 R0～R3、业务或部署验收，未新增 skip 或削弱断言，未提交。
- 最终复核：明确必要证据可靠保存后才对外交付；默认绑定只由既有显式部署装配恢复，知识声明不安装实例；静态帮助与安装账/配置/其它 scope/来源秘密的可见性分开。所见现状差距均保留触发条件，没有把静态疑点写作已证实运行故障。
- 后续收敛：本条的完整 Host/安装生命周期预设及 Object 统一迁移前置已由 INDEX-EXTENSION-PLAN-01 撤回；保留此条当时文档验证记录，当前方案以真实索引的三项贡献接入为准，不计此前或当前任何实现完成。
