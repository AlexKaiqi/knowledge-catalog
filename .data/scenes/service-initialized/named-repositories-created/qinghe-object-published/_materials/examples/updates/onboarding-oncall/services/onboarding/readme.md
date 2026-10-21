---
entity: example/store-oncall
aspect: readme
schema_ref: schema/core/readme/v1
path_hint: services/onboarding/readme.md
---
# 从静态值班资料到 LiveAccess

本导读随 `store-oncall` r3 交付，需已认证身份和 Dataset 消费权。
公开服务的 health 仍是登记时的静态观察，order-cli 也没有预置真实订单服务。
下面的练习在自己的仓使用 `learning/order-service`，不修改公开样例。

```sh
kc dataset show store-oncall --revision 3
kc read 'urn:kc:object:service/qinghe.order-service#/interface' --dataset store-oncall --revision 3
kc read 'urn:kc:object:service/qinghe.order-service#/health' --dataset store-oncall --revision 3
kc read 'urn:kc:object:example/store-oncall#/readme' --dataset store-oncall --revision 3
```

## 先决定维护粒度和取值承诺

本例接口 operations 是整份维护的普通列表；如果每个接口需要按稳定键单独修订，
可以改建 interfaces 成员。不要把“能按数组序号读取”误当成稳定成员身份。
服务与活动是两个对象，以 affected-by 关系连接；runbook 是同一服务的一个方面。

审核后的手册发布快照；允许周期刷新时由接入方采集后经 Writer 发布；
需要读取当时的健康状态才使用 LiveAccess。下面先保存 runtime/health 成员，
再只把这个成员改成外部取值，处置手册（readme 方面）继续静态保存。

## 下载练习材料

准备自己的空练习目录，所有下载目标必须不存在。源程序只使用 Python 标准库。
目录里的对象 YAML 是待发布的原始文件，下载本身不激活任何能力。

```sh
mkdir -p ./live-lab/static ./live-lab/live
kc files read --dataset store-oncall --revision 3 --path tools/live-lab/origin.py --out ./live-lab/origin.py
kc files read --dataset store-oncall --revision 3 --path tools/live-lab/state.json --out ./live-lab/state.json
kc files read --dataset store-oncall --revision 3 --path _schemas/readme.v1.aspect.yaml --out ./live-lab/static/readme.v1.aspect.yaml
kc files read --dataset store-oncall --revision 3 --path tools/live-lab/static/schema.yaml --out ./live-lab/static/schema.yaml
kc files read --dataset store-oncall --revision 3 --path tools/live-lab/static/health.yaml --out ./live-lab/static/health.yaml
kc files read --dataset store-oncall --revision 3 --path tools/live-lab/static/runbook.md --out ./live-lab/static/runbook.md
kc files read --dataset store-oncall --revision 3 --path tools/live-lab/live/descriptor.yaml --out ./live-lab/live/descriptor.yaml
kc files read --dataset store-oncall --revision 3 --path tools/live-lab/live/health.yaml --out ./live-lab/live/health.yaml
```

## 先发布静态成员

`<your-repo>` 是你已获写权的练习仓；`<base-commit>` 来自本次 head，
`<static-commit>` 来自提交回执的 result.newCommit。若 learning 命名空间已被占用，
先一致修改练习文件中的对象、Schema 与描述引用，再使用修改后的读取地址。

```sh
kc writer head --repo <your-repo>
kc writer commit --repo <your-repo> --base <base-commit> --command-id lab-static-1 --dir ./live-lab/static
kc read 'urn:kc:object:learning/order-service#/runtime/health' --repo <your-repo> --commit <static-commit>
```

期望 status 为 healthy，值来自提交，没有外部观察。处置手册单独保存，Schema 只规定健康值结构。
练习的 `schema/learning.health` 约束 runtime 集合的成员；公开服务的 `schema/service.health`
约束整个 health 方面并包含快照时间，不能原样用于这个成员形状。外部观察时间留在依据里，
不为接实时值而在成员的业务值中增加观察字段。
提交响应丢失查原 command-id 回执，不换 ID 重发；并发冲突先重读和比较。

## 接好源，再改变取值来源

源提供者在测试网络运行：

```sh
python3 ./live-lab/origin.py --state ./live-lab/state.json --port 7391
```

程序默认仅监听 127.0.0.1，启动输出实际地址。部署维护者确认 KC Server 可达该地址，
且已有处理器认领 `runtime: onboarding-health`、`protocol: resource-access/v1`。
KC 在容器或另一台机器时，localhost 指向 KC 所在环境；源提供者须安排可达的测试地址，
必要时显式设置 --host，再把 descriptor.yaml 的 origin 改为实际地址。
缺处理器请按部署插件配置接入，不能靠发布 YAML 安装处理器。

该源用于隔离测试，state.json 的 deny 是拒绝演示开关，不是生产身份验证。
真实接入由源验证调用者证明，部署维护者配置精确可信 origin；凭证不写入描述。
本例描述采用 per-caller，不作为共享索引输入。

```sh
kc writer head --repo <your-repo>
kc writer commit --repo <your-repo> --base <base-commit> --command-id lab-live-1 --dir ./live-lab/live
kc read 'urn:kc:object:learning/order-service' --repo <your-repo> --commit <live-commit> --request-id lab-read-1
```

重新取 head 替换基点，`<live-commit>` 用新回执。新 health 文件保持同一成员地址和 Schema，
无静态正文，只引用描述。期望 runtime.health 为 healthy，readme 处置手册仍在，
observations 带该成员地址、声明版本和源观察时间。读取 `<static-commit>` 仍返回静态值。

## 改源值、制造失败、复核证据

保持 `<live-commit>` 不变，依次编辑 state.json，每次保存完整 JSON 后重读：

| 编辑 | 需要核对的结果 |
|---|---|
| status 改 degraded，revision 改 demo-2 | health 变为 degraded，源版本变 demo-2；声明 commit 不变 |
| fail 改 true | health 逐值失败，不能返回旧 healthy；静态 readme 处置手册保留 |
| fail 恢复 false，deny 改 true | health 被源拒绝，不能当合法 null 或无数据 |
| deny 恢复 false | 再读取恢复，先前失败证据不改写 |

需要 audit.read 的身份才能查原结果，消费权不自动包含审计权：

```sh
kc operations audit access --repo <your-repo> --filter-request-id lab-read-1
```

原请求应保留最初采用的 healthy 与观察依据，不能取 demo-2 冒充历史。
练习完成后停止自己启动的源。需要共享时，按系统 Dataset 指南选择文件、发布修订并单独发权；
固定 Dataset 修订仍不冻结外部当前值。真实业务接入前还需验证源侧身份和部署可达性。
