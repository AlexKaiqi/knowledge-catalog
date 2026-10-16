# KC 本地构建与安装包

构建现有 [`cmd/kc`](../../cmd/kc/README.md) 二进制，供使用者连接 KC Server。
同一二进制保留 `serve` 与部署管理入口；安装只放置文件，不启动服务。此入口不包含 `kcfs`，
也不签名、公证、上传或发布。

## 构建

构建机需要仓库要求的 Go、Git、Python 3.11+（使用 `.venv`）；只用 Python 标准库。
没有 `.venv` 时可先执行 `python3 -m venv .venv`，无需安装 Python 包。
四个目标统一产 `.tar.gz` 解压安装包，可从任一支持的构建机交叉构建，无需平台打包工具。

```bash
make build                                      # 本机架构二进制
make build TARGET=linux/amd64                    # 交叉编译二进制
make package VERSION=0.1.0                       # 本机架构安装包
make package TARGET=linux/arm64 VERSION=0.1.0     # 单个 Linux 安装包
make package-all VERSION=0.1.0                   # 四目标
make test-package                               # 不启动服务的打包与安装回归
```

`TARGET` 为 `darwin/arm64`、`darwin/amd64`、`linux/arm64`、`linux/amd64`。
`GO` 可选择已有 Go 可执行文件，`PYTHON` 可选择已有 Python；`DIST_DIR` 默认 `dist`。
`VERSION` 默认 `0.0.0`，接受三个无前导零的数字段，例如 `0.1.0`。本地开发包由随包记录的源码
revision 和 dirty 状态进一步辨认，不把任意 Git 描述直接当系统包版本。

| 入口 | 产物 |
|---|---|
| `make build` | `dist/bin/<os>_<arch>/kc`、`build-info.json` |
| 安装包 | `dist/kc_client_<os>_<arch>.tar.gz` |
| 打包校验 | `dist/SHA256SUMS`，列出本次成功打包的所有安装包 |

二进制关闭 CGO、移除源码绝对路径，保持 amd64 v1 / arm64 v8.0 基线。安装端无需 Go 或 Python。
`build-info.json` 记录版本、Git revision、是否有未提交改动、目标、Go 工具链及二进制 SHA-256。
包内不收集工作区配置、凭证或会话。打包使用独立临时目录；本次所有目标完成后才替换安装包与校验清单，
构建或打包失败保留原安装包。已有其它版本产物不清除，`SHA256SUMS` 仅覆盖最后一次成功打包。

## 安装与卸载

先在产物目录校验，再选择与系统、CPU 相符的包：macOS 用 `shasum -a 256 -c SHA256SUMS`，
Linux 用 `sha256sum -c SHA256SUMS`。包没有签名；SHA-256 用于核对本地交付文件，不提供发布者认证。
macOS 与 Linux 安装方式一致：解压后执行安装脚本；脚本默认安装到 `/usr/local`，需要写入权限：

```bash
mkdir kc-install
tar -xzf dist/kc_client_darwin_arm64.tar.gz -C kc-install
sudo sh kc-install/install.sh
kc help
sudo sh /usr/local/share/kc/uninstall.sh
```

也可安装到自己的目录：

```bash
PREFIX="$HOME/.local" sh kc-install/install.sh
"$HOME/.local/bin/kc" help
PREFIX="$HOME/.local" sh "$HOME/.local/share/kc/uninstall.sh"
```

安装到 `/usr/local/bin/kc`，元数据与卸载脚本位于 `<PREFIX>/share/kc/`。
`PREFIX` 必须是非空绝对路径，安装前检查系统与架构。安装支持覆盖升级，`kc` 用同目录临时文件替换；
脚本不自动执行 sudo，不修改 PATH 或 shell 配置。按需把所选前缀的 `bin` 目录加入 PATH。
安装与卸载保留 KC 连接配置和登录会话；登录操作见 `kc help login`。

## 验证边界

`make test-package` 检查打包目标、内容、校验、失败路径及临时目录内的安装、升级和卸载，
也由 `make check-validation` 执行。实际四目标编译、原生包内容检查和本机执行分别记录，
交叉编译成功不等于其它系统的安装运行验收。协议与真实部署验证仍使用
[原验证入口](../../doc/engineering/testing.md)。
