# home/

打开 `--home`、发现 Catalog/成员、选择 Snapshot authority、装配 Writer / Reader / ControlPlane / Index。这是 A-01 的 composition root：具体 Dolt/Gitea adapter 只允许在 `authority_drivers.go` 被 import。

本包不得 import `cli`、`client` 或 `httpsurface`。公开业务 CLI 仍经 Server；只有 `kc local` 与 `kc serve` 打开 Home。

| 文件 | 负责 |
|---|---|
| `home.go` | 装配活对象图：Store、Catalog、Writer、Reader、Index |
| `home_discover.go` | 扫磁盘布局；目录即真相，没有第二份清单 |
| `home_mount.go` | Repository attach 编排；不是 `kcfs mount` |
| `home_sidecar.go` | AfterSnapshot → Index、Merge Gate |
| `home_system.go` | 内置 `kr://kc/system` 发布与校验 |
| `journal.go` | 把过程账转发到 Writer / Reader / Catalog / ControlPlane |
| `authority_drivers.go` | 唯一允许 import 具体 Dolt/Gitea 的文件 |
| `stores.go` | 配置契约、默认值和路径名 |
| `stores_file.go` | `layout.yaml` / `stores.yaml` 读写及旧格式迁移 |
| `stores_profile.go` | profile、driver、目录归一化和校验 |
| `stores_flags.go` | `store-set` flags / DSN 翻译 |
| `stores_public.go` | 不含密钥的公开状态视图 |

argv、HTTP handler、授权求值和出站 Hook 仍在 `cli/`。
