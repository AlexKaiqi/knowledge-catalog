# 清河茶铺例子世界

一家茶铺，六个团队各自登记自己的对象，再按问题组成三个公开 Dataset。任何已认证的人都能读这三个
Dataset；来源仓本身不公开，也不能被读者改写或重新发布。

## 六个仓

| 仓 | 谁维护 | 登记什么 | 怎么接入 |
|---|---|---|---|
| `product` | 商品组 | 商品桂花乌龙、业务概念“订单”（门店、数仓、下单服务三种说法） | 托管 |
| `campaign` | 市场组 | 2026 秋季上新活动：推广哪个商品、用哪条素材、首日提示 | 托管 |
| `media` | 内容组 | 15 秒竖版成片 mp4、封面 png、中文字幕 vtt 及其说明 | 自有 lakeFS |
| `store-ops` | 交易组 | 下单服务（接口、健康、处置手册）、命令行工具 order-cli、p99 告警 | 自有 lakeFS |
| `table-meta` | 数仓组 | 订单、明细、商品等表与字段、加工作业 | 托管，材料在 `../table-meta` |
| `sales-semantic` | 分析组 | 语义模型、GMV 等指标及口径 | 托管，材料在 `../sales-semantic` |

跨仓引用直接写对象 ID：活动的 `productRef` 指向 `product/qinghe.osmanthus-oolong`，素材说明也指向它，
商品又指向数仓的 `table/shop.products`。原样文件（视频、图片、字幕、脚本）不经 Writer 改写，按字节进入
接入方自己的 lakeFS，对象只描述它们。

## 三个 Dataset 与用例

| Dataset | 谁在问 | 原话 | 拿到什么 | 之后的变化 |
|---|---|---|---|---|
| `qinghe-analysis` | 分析组 | 上周销售额为什么下降？ | 表与字段、作业、GMV 口径、商品与“订单”概念 | r2：9 月 14 日起 GMV 计入 UNPAID；r1 仍按旧口径可复查 |
| `autumn-launch-kit` | 内容组、渠道 | 秋季上新的视频素材能在哪些渠道用？ | 成片、封面、字幕、使用范围、活动与商品 | r2：字幕“28 元”更正为“32 元”；r3：识图团队补上画面描述 |
| `store-oncall` | 值班 | 下单服务告警了，先做什么？ | 服务接口与处置手册、order-cli、告警、活动首日提示 | r2：手册加上“先看活动首日提示，只有延迟升高时不要重启” |

收尾问题是店长的“桂花乌龙秋季活动效果怎么样”：活动的成功标准指向 `metric/shop.gmv` 的当前口径
（`qinghe-analysis`），活动和素材在 `autumn-launch-kit`，首日延迟告警的来龙去脉在 `store-oncall`。三个
Dataset 都按修订号固定，答案能说清自己基于哪一版。

如实说明：`store-ops` 里服务的 health 是登记时的静态快照，不是实时探测；order-cli 只演示形态。
下载并加执行权限后，`./order-cli --help` 可离线查看版本、用法和配置要求；执行 health 或 get 时，没有
`ORDER_SERVICE_URL` 则以退出码 3 结束。材料不预置真实订单服务或业务认证配置。

三个 Dataset 还在 `_schemas/` 交付所选对象采用的 Schema，同一 Schema 身份只选一个来源。
先用 `kc show` 选择公开例子，再固定同一 revision，经 `kc files list`、`kc files read` 取得 Schema
和对象原文件，学习文件头、身份与正文的对应关系；完整复用步骤见系统的首次发布指南
`kc read 'urn:kc:object:kc/guide/first-publication#/guide' --repo kr://kc/system`。
公开消费权只覆盖 Dataset 选中的内容，不授予来源仓读写权。复用的是结构和表达方式；示例中的价格、
口径与健康状态是该示例的声明，自己的资料应使用自己的对象身份、真实正文与来源。

## 文件

- `world.json`：发布计划的唯一来源，列出六个仓的接入方式和按顺序执行的步骤。
- `repos/`：各仓基础对象，含 `drafts/` 下的 Schema。
- `files/`：原样文件，按仓和修订分目录。
- `updates/`：后续修订，每个目录只含本次变化的对象。
- `datasets/`：三个 Dataset 配方，带 title、question、owner 元数据。
- `deployment-access.yaml`：部署配置里的 `datasetAccess` 片段，把三个 Dataset 声明为已认证默认可读。
- `publish.py`：发布脚本，只用 Python 标准库。

## 发布

前提是 Server 已经部署并运行，部署配置已合入 `deployment-access.yaml`，`media` 与 `store-ops` 两个
lakeFS 仓已在接入方 lakeFS 上建好。脚本不启动任何服务。

```bash
export KC_SERVER_URL=https://kc.example
export KC_AS=agent:operator                 # 或先 kc login
export EXAMPLES_LAKEFS_URL=https://lakefs.example
export EXAMPLES_LAKEFS_CREDENTIAL_FILE=~/.config/qinghe/lakefs.credential   # 一行 accessKey:secret
python3 publish.py --plan                   # 只看还剩哪些步骤
python3 publish.py
```

脚本可重复执行。Writer 步骤用固定命令 ID，Dataset 步骤用固定修订号；Server 能核实的最后一个已完成
步骤之后才继续，所以重跑不会把旧字幕重新提交到新修订之上。Writer 会把 `--dir` 原样记进对象来源，
脚本因此在本目录下以相对路径提交，不把发布者的本机路径写进公开内容。

## 核对

先选定 Dataset 修订，再读素材对象的说明，从返回的 `value.file`、`value.cover` 和
`value.subtitles` 取得成片、封面和字幕的路径。这个 Dataset 保留了这些文件的原路径；下载沿用
对象读取时的同一修订。完整调用合同分别见 `kc help read` 与 `kc help files read`。

```bash
kc dataset show autumn-launch-kit --revision 3
kc read 'urn:kc:object:media-asset/autumn-launch.cut-15s#/definition' --dataset autumn-launch-kit --revision 3
# 本例 value.file、value.cover、value.subtitles 依次返回下面三个 --path；--out 目标须不存在。
kc files read --dataset autumn-launch-kit --revision 3 --path assets/autumn-launch/cut-15s.mp4 --out cut-15s.mp4
kc files read --dataset autumn-launch-kit --revision 3 --path assets/autumn-launch/cover.png --out cover.png
kc files read --dataset autumn-launch-kit --revision 3 --path assets/autumn-launch/subtitles.zh.vtt --out subtitles.zh.vtt
```

核对字幕更正时，分别固定旧版和新版；旧版仍保留原文。

```bash
kc files read --dataset autumn-launch-kit --revision 1 --path assets/autumn-launch/subtitles.zh.vtt   # 28 元
kc files read --dataset autumn-launch-kit --revision 2 --path assets/autumn-launch/subtitles.zh.vtt   # 32 元
kc read 'urn:kc:object:product/qinghe.osmanthus-oolong' --dataset autumn-launch-kit --revision 3
```

同一计划在 `cli/example_world_test.go` 里进程内跑通：任意已认证身份先读取各修订的素材说明，沿正文
路径下载同版视频、封面与字幕，并与原文件逐字节核对；交付的脚本也保持原字节。各修订内容如上表，
来源仓 `--repo` 读取与重新发布被拒，整份计划重跑不产生新提交。真实部署上的发布仍需按上面的步骤
单独执行。

`cli/example_tool_journey_test.go` 复用同一发布计划，从工具对象说明取得脚本路径，固定修订下载后执行
离线帮助、公开启动命令和两个查询例子，并核对实际请求及响应。查询目标是测试进程内的模拟订单服务；
这不代表真实订单系统、陌生 Agent、MCP 连接或 Skill 安装已验收。
