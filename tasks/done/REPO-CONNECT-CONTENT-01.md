# REPO-CONNECT-CONTENT-01 · 远程仓接入在系统仓的说明补齐

## 目标

接入远程仓（自有来源连接、托管供给、登记、解绑）这一条线上，系统仓能支撑起可判定的操作：读者能区分"创建托管仓"与"连接已有仓"、知道凭证是服务私有且可轮换、知道登记失败不留下半个登记、知道来源离线与登记丢失是两种不同故障。

当前缺口：`capabilities/repositories/definition.md`（655 字节）只写"创建托管仓、连接已有来源或解除登记"，`usage.md`（565 字节）与 `guides/connect-source/guide.md`（908 字节）都停在"核对服务地址、按回执核对、创建成功不等于已发权"。`snapshot.md` 里已有的"供给新仓、连接已有仓、登记、授权是四件不同的事"以及"凭证可以轮换""登记失败不留下半个登记""来源离线和登记丢失要区分"，都没有出版到系统仓。系统仓内没有出现"自有仓""托管仓""凭证轮换"这些接入方实际要用的词。

## 依据

- [snapshot](../../doc/design/snapshot.md)：登记、供给与连接；托管仓与自有仓的差别、凭证可轮换、登记失败的恢复要求。
- [服务](../../doc/design/service.md)：服务私有凭证不下发；初始化是显式管理动作，启动只恢复。
- 系统仓说明分类守卫 `systemrepo/system_explanations_test.go`：`capabilities` 与 `concepts` 的正文须有已复核的既有系统依据，不得借此新增领域概念。

## 验收

- 系统仓出现可判定的接入说明：托管仓与自有仓的选择依据、凭证归属与轮换、登记与授权是分开的两步、登记失败的后果、来源离线与登记丢失如何区分。每条都能指回 `snapshot.md` 或 `service.md` 的既有表述，不自创合同。
- `kc attach`、`kc create`、`kc detach` 三个调用入口与正文所述任务一致；`capabilities/repositories/entrypoints.yaml` 不改动作名。
- `go test ./systemrepo/...` 全绿（含依据守卫与导航闭包）；`make check-docs` 通过。
- 抽取验证：只读系统仓的人能正确回答"连接自有仓失败后能否重试""解除登记是否删除源仓数据""凭证由谁持有"三个问题。

## 前置

无。

## 不做

不新增 connect/connection 之外的命令或动作；不改凭证存储与轮换的实现；不把部署配置字段清单抄进系统仓；不承诺尚未支持的远程介质（如 DVC、远程 Dolt）。

## 交付（2026-10-20）

- 补齐仓接入的能力定义、用法与 connect-source 指南：托管与自有来源的选择、create/attach/授权的分离、
  凭证私有保管和轮换、分阶段恢复、登记完整性、来源离线与登记丢失、detach 后果。保留原对象身份、
  Schema、导航关系与 entrypoints 动作名。
- 正文按 `snapshot.md` 的“登记、供给与连接”及“恢复”、`service.md` 的“恢复”及“部署与认证”核对。
  具体调用对照 create/attach/detach help、Home 连接合同与 HTTP 连接管理入口。
  create 叶子 help 补明当前凭证文件格式；轮换指向已有 API，不引入 CLI 命令。
- 独立阅读抽查通过：读者能说明连接失败后按原请求恢复的条件、detach 不删除来源数据、
  凭证由服务私有持有及连接所有者负责轮换。抽查发现的格式说明和轮换入口缺口已补齐。
- `go test ./systemrepo/... -count=1`、三条 CLI help 回归及 `make quality`（含 check-docs）通过，无 skip。
  日志在 `.validation/remaining-20261020/repo-connect-{systemrepo,help,quality}.log`。
- 本轮门禁同时清除已有阻塞：删除无调用的 `datasetSpace.requiresState`；架构守卫以 go/types 保留作用域身份，
  并验证遮蔽与闭包；搜索续页校验提为私有函数，复杂度 52 降至 47，预算、续页和取消回归通过。
  没有调整门禁阈值或削弱原断言。
- 未提交；只在本地验证，没有部署到开发机。
