# 已配置但未接入的业务源

construct 准备仓与关系仓的既有 Snapshot binding，确认 登记空间 仍只登记 System Repository。两仓前态由未登记边界 probe 与后续 attach 共用。System 信任根已在初始化时提供；Schema 浏览、正文回读与不可写验证挂在初始化前态。

仓 `kr://scene/object` 的来源 authority 原生 `kc.plugins` 明确选择
`liveaccess/main`；已有索引选择保留，关系仓不因此选择访问插件。`Given existing
repository ... with LiveAccess instance main` 是墙外来源配置前态，不是新增 CLI
参数。Go 夹具和真实 lakeFS 场景重放都在绑定前写入同一原生 metadata，再冻结或
接入。实时源无需已在线，后续离线访问用例仍验证来源不可达的原错误。

已有部署的来源管理者须按 `discovery/adoption` 的 Repo 配置合同在该 authority
上完成同一选择，并在服务启动前提供。`attach` 只验证并登记来源，不代替其插件配置。
当前 `goto` 没有读取原生 Repo metadata 的 CLI 接缝，因此遇到这句显式采用前态
会在预检阶段明确失败，不执行构建、不报告 ready；真实 lakeFS 场景执行器能构建并
核验该前态。后续 walk 若要接入本状态，须补上真实核验入口，不能跳过本句。
