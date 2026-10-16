# SNAPSHOT-CLI-02-IMPLEMENT · 第二批 Snapshot CLI 实现（2026-10-13）

- [x] 本轮完成：实现已选定的 `dataset show`、`files list/read` 与 `grant update`，补历史版解析、交付目录混排分页与真实 argv 回归，受影响测试、`make check-docs`、`make quality` 全绿；无新 skip，未提交。
- Goal / owner：交付 Dataset 详情/历史、单目录页和单文件消费、同一授权规则的条件原子修改；设计沿 `doc/design/affordance.md`、`dataset.md`、`service.md`、`permissions.md` 及上一条合同选择，owner/直接关系已按 docs/graph 读取。
- Non-Goals：保持 `cli` 单包、旧命令及既有消费默认，不扩对象层、不新增原生仓属性入口、不增加 pin 文件或会话、不整改或迁移独立 kcfs / datasetfs、不承诺来源 Range IO、不安装依赖、不启动部署服务、不提交；保留其它会话全部在途修改。
- 不变量：V-01、KS-01/KS-02、AUTH-01～03、API-01、A-01/L-01；版本从已接受记录重建，当前退役与权限仍检查，交付路径不扩大范围，目录页严格可续，字节不改写，目标不覆盖，规则身份与来源保持且旧/新范围在原子边界内鉴权。
- 选定：补现有 named Resolve 的显式发布版选择与 typed Client 详情查询；目录混排分页在网关完成且游标绑定发布/目录；CLI 复用文件网关，以 JSON 字节块或完整单文件目标交付；授权复用已有原子 Update，输入两份单条 Rule 文件并做规范传输转换。否决管理详情拼 pin、CLI 全量分页再截断、二进制 stdout、自动刷新 expected、remove/add 替代原子修改与新 CommandSpec 框架。
- 接口与代码：Snapshot 文件/历史/授权合同仍由 `snapshot/dataset`、`snapshot/service`、`snapshot/access` 与 Client Go 类型/包 README/Conformance 拥有；CLI 落点沿已选文件簇、surface/分派/应用登记及同源 help 入口，旧形状保持。
- 验证：先取得新增需求的失败证据再实现；定向 CLI argv、Client、Dataset、Access、Service、架构/命令/帮助闭集、发布与撤权、二进制/目标保护及 race 检查；之后文档和工程质量闸全绿，无新 skip。未执行的真实部署不记验收通过。
- 质量出口：增量问题当场修或升级守卫；既有整文件读再切片成本债沿合同条保留。当前其它线程正在 Object R0 / 索引采用工作，本批不认领、修改或将其结果计为新增 CLI 证据。
- 实现范围：四个叶子已接 typed Server，代码保持 cli 单包，详情、文件、规则输入分别收拢为同包文件；root / 组 / 叶 help 同源。显式历史版只重放已接受的冻结提交，跳过默认配方首次采用；省略版本的原行为保留。交付分页先派生直系子项、再按剩余额度读取一个来源目录页，游标绑定发布、目录和当前可见范围，不缓存权限或全量扫描来源。
- 增量复核：修正显式历史请求可能意外创建首个发布、无效输入可能先刷新登录、下载响应来源未属于 pin、跨块来源切换及目标创建竞态；均进入必要回归。下载同一来源和总大小，失败清理临时字节，完整后以同目录 create-if-absent 发布。规则输入拒绝未知/重复/null 字段并显式转换 repo，更新不拉新 expected、不改规则身份与来源。
- 当前验证记录：Client、Snapshot Access、Dataset、Service 四个整包 race 已通过，331 个终态测试事件、fail / skip 为 0。受影响 CLI 当前选择为 65 个主测试、177 个通过事件，整包 race 通过且无 fail / skip；包括新入口、旧命令/帮助闭集、真实 argv 发布/历史/混排消费、原子撤权、二进制下载及既有目标保护。最后调整目标创建竞态的错误分类后，12 个文件相关事件再次 race 通过。Snapshot 及所选文档/库存守卫另有 27 个通过事件，受影响包 vet / staticcheck 与实际 kc help files 通过。
- 失败证据与发布：历史显式解析的采用漏洞和跨块合法来源切换均由旧行为 overlay 取得真实反例；目标创建竞态此前错报参数错误，现统一为既有目标冲突并保留并发写者内容。现有同源生成器已刷新四个入口，仅 datasets / inventory / permissions 三份出版物增量；生成器沿并行迁移后的 systemrepo/content 落点，未重建退役目录。共享工作树的 Object 迁移过程曾阻挡编译、系统出版、文档和质量闸；这些失败保留日志，不计通过，不修改其它迁移或弱化断言换绿。
- 收尾进展：文档闸已恢复为 32 篇 / 149 关系通过，运行前后源码未变化。消费回归加强为从 show 获取 ID、从 dataset show 获取交付路径，再枚举/读取同一发布；旧 --datasets 不是现有公开参数，已纠正测试。质量闸进一步发现本批直连 snapshot/service 增加了 CLI 依赖和 service 不稳定度漂移，现改用 Invocation 已有的同一纯路径校验合同，不复制校验或增添公共形状；后续重跑受影响验证。其它迁移的单位登记与传播成本仍由对应任务处理，不放宽守卫或基线换绿。
- 最终验证：迁移恢复后重跑受影响 CLI 的 65 个主测试（177 个含子例的通过事件）及 Client / Snapshot Access / Dataset / Service 四个整包（331 个通过事件），均为 race、fail / skip 为 0；加强后的无预知 ID / 路径消费旅程在其中真实通过。Snapshot 架构选择 26 个事件、Server 文件接线选择 6 个事件及文档/验证库存包 28 个事件通过，均无 fail / skip。定向范围不替代完整 Object 架构、全仓业务合同或真实部署验收。
- 最终闸证据：`make check-docs` 的 gate-watch-10 与 `make quality` 的 gate-watch-06 均退出 0；32 篇 / 149 关系、格式/tidy/vet/staticcheck/复杂度/文件体积/克隆/耦合全绿。两项使用同一候选，1359 个 Go / Markdown / OKF 与关键输入在运行前后及结束复查均未变化。本批自己的 service 耦合漂移已消除；并行 Owner 对迁移单位口径与基线的调整不计作本批实现或质量修复。原始日志沿 `/tmp/kc-snapshot-cli-phase2-*`；记录回写后再跑文档与质量闸。
- 交付边界：CLI 保持单包，既有 57 个路径保留、新增四个所选叶子；独立 kcfs / 根 datasetfs 不迁移。无 pin 文件/消费会话、对象入口、原生仓属性入口、依赖安装、部署服务或本批提交。来源整文件读再切片的成本债及触发条件沿上条保留。
