# RESOURCE-METADATA-01 · Repo 原生元数据与 Dataset 可选元数据（2026-10-13）

- [ ] 本轮认领：设计、公开合同与实现一起支持 Repository / Dataset 的可选自定义元数据；README 不成为创建、发布或发现的前提。
- Goal / owner：Dataset 的资源说明与版本交付归 `doc/design/dataset.md`，原生 Repository 接入与服务边界归 `doc/design/service.md` / `core-architecture.md`，README 与知识说明归 `knowledge.md`，授权归 `permissions.md`；按 `docs/graph/` 的 owner 与直接关系定位。
- Non-Goals：不建立第二份 Repository 元数据权威，不把资源属性解释为知识或授权，不强制 README / Schema，不引入 PATCH、跨仓事务或新的搜索代数，不安装依赖、不提交、不运行真实部署验收。
- 不变量：保留 V-01、KS-01 / KS-02、AUTH-01～03、API-01 与 A-01 / L-01；Dataset 已接受发布（含元数据）不可改写，元数据不改变文件范围、来源身份或权限。
- 选定：两种资源采用 lakeFS 原生的字符串键值形状；Repository 读写使用原 lakeFS metadata API 与权威，Dataset 可在定义和新版本发布时携带完整可选元数据，并随发布记录持久、恢复、回读。无元数据继续成立；README 是可选详细说明。否决复制原仓 metadata 到登记账、强制选入 README，以及把 metadata 放入知识 Schema 或索引作为正文权威。
- 接口：Repository 沿用 `snapshot/service` 的原生受控路由、`snapshot/access` 动作和控制台；Dataset 以 `snapshot/dataset.KnowledgeSet` / `KnowledgeSetRecipe`、Snapshot HTTP / Client 与定义控制台承接，具体字段与校验写在公开类型、包 README 和回归合同。
- 验证：执行受影响 Dataset 持久化/历史/深拷贝/配方、原生 Repository 读写授权与代理、Client/CLI/HTTP 和前端回归，以及 `make check-docs`、`make quality` 与受影响合同；不以定向测试宣称真实部署或全量验收。保留本轮开始前所有工作树改动。
- 质量闭环：本轮发现的增量问题当场修或补守卫；存量信号按位置 / 信号 / 判定 / 处置 / 触发条件记在本条，未完成验证不勾选。
- 实现范围：原生 Repository metadata GET / POST / DELETE 与显式读写授权、Snapshot SDK 和原仓设置页；Dataset 定义、便携配方、发布准备、持久恢复、库存、历史详情、CLI `--metadata` 与表单 / JSON 编辑。两者都使用字符串键值，元数据能力完全不读取、识别或处理 README。
- 增量问题处置：修复空 overlay 与状态/返回值共享 map、发布准备及配方入口丢元数据、嵌套定义绕过类型校验；严格校验集中在纯函数与传输入口，保留公开 DTO 纯数据架构守卫、业务任意值和精确数字及 Writer 流式路径。增加持久旧发布不可改写/删除与失败保持回归；原生客户端补错误分类和原生请求不压缩合同，UI 部分失败回读实际权威。
- 同期门禁信号：原生物理来源源码注释使用 CLI 守卫的保留词导致文本误报，仅将注释改为同义 forwarding，未改逻辑或守卫；前端同期文件读取改动遗留未使用变量，仅清理废弃项并由现有 lint 验证。未覆盖其它在途功能。
- 验证过程：前端 172 项与 typecheck / lint / build、受影响 Go 与 Dataset race、文档和工程质量闸已有通过记录；首轮普通场景业务通过但 sourceChanged=true，不作为同份源码证据。完整契约发现公开 DTO 解码方法违反守卫，已改为纯验证端口并重跑相关架构回归到绿；最终普通场景与完整契约在固定候选上复跑，真实部署验收保持独立、未执行。
- 最终验证：固定候选指纹 `f6cae9f1905dce00ba100c13a018ac33f868863b16cbe70b50416516026deb40`；`make test` / `20261013T032225Z-2d3b133a08c3` 退出 0，21 pass / 0 fail / 0 skip；`make test-contracts` / `20261013T032225Z-74b7acaf2e5a` 退出 0，2229 pass / 0 fail / 11 既有外部 skip，CLI 57/57 成功且满足风险分层合同。两次 sourceChanged=false；pass 事件含子测试，不作为库存通过率。固定候选 `make test-webui`（175 tests、typecheck / lint / build 与控制台 Go 回归）、`make quality` / `make check-docs` 全绿；当前工作目录另跑元数据 / Native / 架构 / 精确解码回归与 `make quality` 到绿，未覆盖同期在途变更。
- 未验证边界：上述 11 项为既有 live runtime、真实 lakeFS、身份、付费模型和规模合同，与本轮新增回归无新增 skip；真实部署验收未执行。实现完成，但依严格「完整契约无 skip」勾选规则保留本条未勾选，不记全量 / 真实部署验收通过。固定候选证据与日志已保存在 `.validation/`；只回写交付记录，没有提交。

## 关闭

2026-10-15 拆分 `TASK.md` 时关闭，未按原条目验收。早于 2026-10-14 对象模型重构的认领，相关代码与设计已被后续重构改写（见 `0759640` 之后的提交），原范围按旧结构写成，不再继续。整体验收并入 `FINAL-ACCEPTANCE-01`，插件相关用例由验收规格 accept-01～13 取代。
