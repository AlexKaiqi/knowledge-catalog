# home/config/

生产部署配置的形状、解析与静态校验。读取一份部署文档不打开 authority、不构造索引或
LiveAccess 引擎、不写部署状态、不启动维护；打开来源、恢复资源和装配对象图归根包 `home/`。

本包不依赖 `home`、`home/repositories`、`server`、`invocation`、`cli`、`client`。
`internal/arch/layers_test.go` 的层表与禁止依赖规则显式登记这两条边界。

## 内容

| 文件 | 职责 |
|---|---|
| `deployment.go` | `DeploymentConfig` 形状、`ReadDeployment`、`Validate` 与绑定/证据校验 |
| `stores.go` / `stores_file.go` | `StoresFile`、`LayoutFile` 形状、默认值、profile 与磁盘读写 |
| `drivers.go` | 驱动名折叠与绑定校验；只持有纯校验，不持有打开能力 |
| `managed.go` / `access.go` / `dataset_access.go` | 托管供给、仓访问与 Dataset 访问的声明与纯校验 |
| `connections.go` / `auth.go` | 连接策略与退出地址的纯校验 |
| `repository_id.go` | 仓库身份折叠与目录编码 |

## 认证方式

`Validate` 用 `identity.Lookup` 判定 `--auth`，接受的名字来自 identity 注册表。因此
`deployment.go` 链接 `kc/identity/builtin`，它是唯一登记的内置认证器装配根；
`internal/arch/identity_authenticator_test.go` 的装配根列表包含该文件。删除这个链接会让
`local` 与 `taihu` 一并被拒绝。

## 引擎配置

`StoresFile` 内嵌 `opensearch.Config` 作为部署要选的服务端参数。这是声明，不是引擎：
本包不调用任何索引实现，只命名形状。`internal/arch/declarative_index_module_test.go`
把 `home` 与 `home/config` 并列为允许命名该形状的装配点。
