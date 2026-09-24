# 第一阶段部署的本地代替

生产陪伴清单见 [服务边界](../../docs/reviewed/service.md) §11.1。这里只提供同构的本机替代，不是容量资格。

| 生产 | 数量 | 本地代替 |
|---|---|---|
| kc-server | 1 容器 | 本 compose 的 `kc-server` |
| lakeFS | 1 容器 | `treeverse/lakefs:1.86.0` |
| PostgreSQL（只给 lakeFS） | 1 容器或托管 | `postgres` 容器 |
| COS | 托管 | MinIO（仅本地；生产不要起） |
| OpenSearch | 1 容器 | 单节点 OpenSearch |
| 可观测（OTel Collector + 指标 + trace + 日志 + 面板） | 1 容器或现成平台 | `grafana/otel-lgtm` |

自管生产是 **5 个容器 + COS**。PostgreSQL 和可观测都托管则是 **3 个容器 + 托管件**。本地多一个 MinIO，因为 COS 没有本机进程。本地另起 OpenSearch Dashboards、MinIO Console、ttyd，只给操作员看/敲命令，不算生产陪伴件。

`scripts/system-lakefs.sh` 仍是 adapter 实验（lakeFS 内嵌 KV、`kc serve` 在宿主机）。本目录才是准备上线时要对齐的容器拓扑。

## 可观测要不要打成一个镜像？

**本地/首发单实例：要，用现成镜像，不要自己糊。**

- 合适：`grafana/otel-lgtm` 已经把 OTel Collector、Prometheus、Tempo、Loki、Grafana 放进一个容器。KC 的合同是 OTLP traces/logs + 抓取 `/metrics`，不绑定 Jaeger 进程数。本地 trace 走 Tempo 可以。
- 不合适：把这五个进程用 supervisord 打进自建镜像并当成生产 HA；把可观测打进 `kc-server` 镜像。
- 生产：优先把同一 OTLP 接到现成平台（只自管 OTel Collector，或完全托管）。长期保留、对象存储、租户隔离仍由部署方定，见 [服务边界](../../docs/reviewed/service.md)。

产品部署用 `grafana/otel-lgtm` 一个容器覆盖 OTLP、指标、trace、日志和面板；不要再复制一套五进程 Compose。

## 两套隔离的 compose 项目

同一份陪伴清单、同一份 `compose.yaml`，两个项目名和两套卷。清测试栈碰不到开发数据；两套可以同时跑。

| | local 测试 `kc-deploy-local` | 本机开发 `kc-deploy-dev` |
|---|---|---|
| 用途 | 走查、`deploy-local-scenes`、随时从空开始 | 建仓/发权/写知识后重启还在 |
| 端口清单 | `/tmp/kc-deploy-local`，清盘即丢 | `$HOME/.kc/deploy-dev/compose.env`（值就是下表固定口） |
| `down` | 默认清卷并删 env | 只停容器，卷和 env 留下 |
| 清盘 | 与 `down` 相同 | `make deploy-dev-reset` |
| 重启策略 | 默认不拉起 | `compose.dev.yaml`：`unless-stopped` |
| 端口 | 每次从空选空闲口 | **固定**；被占则失败，不换口 |
| scenes | 只挂在这套 | 拒绝 |

必须一起留的是 `kc-data`、PostgreSQL 和 MinIO。OpenSearch 与可观测卷开发栈顺便留。两边都不持久 `/var/lib/kc/cache`。宿主机只持久化 compose.env，数据面用 Docker 命名卷，不 bind 进仓库。开发栈镜像标签是 `kc-deploy-dev:local` / `kc-deploy-dev-cli:local`，重建不会改写正在跑的 `kc-deploy-local` 镜像。

开发栈页面绑 `0.0.0.0`，用主机 IP 访问；ttyd 有 basic auth，不是无认证 shell。local 测试栈仍只绑 `127.0.0.1`。被其它进程占用则 `up` 失败，不会改口。

| 面 | 端口 |
|---|---|
| ttyd | 7692 |
| kc-server /console | 7385 |
| lakeFS | 18010 |
| MinIO S3 | 19010 |
| MinIO Console | 19021 |
| OpenSearch | 19210 |
| OpenSearch Dashboards | 15611 |
| Grafana | 7310 |
| Prometheus | 19190 |
| OTLP HTTP | 14328 |

## 入口

只把 **ttyd** 当作敲 `kc` 的页面。产品观察前端是同一台 `kc serve` 的 `/console`（Catalog、Dataset、检索投影、Snapshot 权威）。根路径 `/` 仍是 404。远程 Cursor 只把**当前这套**访问卡上的 ttyd URL 当敲 `kc` 入口。

```bash
# 可清盘的测试栈（down 默认清卷）
make deploy-local-up
make deploy-local-status
make deploy-local-access
make deploy-local-smoke
make deploy-local-down

# 可重启的本机开发栈（down 保卷）
make deploy-dev-up
make deploy-dev-status
make deploy-dev-access
make deploy-dev-smoke
make deploy-dev-down
make deploy-dev-reset
```

`deployment init` 只建 Catalog 登记表和 System 信任根，不创建业务知识 Snapshot。lakeFS 启动时只有 `kc-catalog` 与 `kc-system`。业务仓由 `kc create --name` 从 `managedStores.lakefs` 分配。Graveler 名、协议 `--repo` 都是 `--name`（例如 `table-meta`）。对象前缀是 `{root}/{name}`（本机 root 为 `s3://kc-authority/tianqiong`，所以 `table-meta` 的 namespace 是 `s3://kc-authority/tianqiong/table-meta`）。`--name` 必须是 lakeFS 允许的小写字母、数字和连字符。

ttyd 预置 `KC_SERVER_URL` 与当前 `kc`。在里面：

```bash
kc login --mode local --as admin
kc whoami
kc catalog list
kc schema list --repo kr://kc/system
```

观察台（浏览器，不是 CLI）：`http://127.0.0.1:<kc-server>/console`，本地登录用户名 `admin`。lakeFS / MinIO / OpenSearch Dashboards 是陪伴件控制台，不是 Dataset 和统一索引这一层。

地址和账号以对应 profile 的 `up` / `status` / `access` 打出的访问卡为准（含 lakeFS Access Key ID / Secret、MinIO、OpenSearch Dashboards、Grafana 面板）。测试栈每次清盘后端口可能不同；开发栈用上表固定口。这些是本机代替默认值，不是生产密钥。

协议旅程同一棵树在这套拓扑上跑（真 Graveler + OpenSearch）：

```bash
make deploy-local-scenes  # 真实部署场景；需先显式 deploy-local-up
make deploy-local-goto NODE=qinghe-knowledge-published PROBE=probe-publish-dataset-with-scoped-members.feature
```

`make deploy-local-scenes` 是独立的真实 lakeFS 部署场景入口，也包含在显式 `make test-all` 中。测试栈由调用者显式准备，测试不隐式重建或清盘。默认 `make test` / `make test-lakefs` 使用原有 lakeFS HTTP 夹具和真实 OpenSearch，不依赖本部署栈。组件/应用混合套件保留为 `make test-contracts`；三者不是相同覆盖范围。人手走查继续走 ttyd → `kc-server`。`make deploy-local-scenes` 可连跑：live Graveler 仓名按次唯一，测完删除。开发栈拒绝 scenes，避免 live DFS 写进长期权威。动态观察由独立 Go 用例配合 State runtime 验证，不再作为可跳过的场景节点。
