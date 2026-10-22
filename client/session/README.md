# 调用方持久会话

本包唯一拥有按服务隔离的登录材料、local/token 配对、浏览器 PKCE/PAR、
验证后持久化、刷新和注销。Configure 返回 [认证运输](../auth/README.md) 配置，供宿主组合 SDK；
每次请求重新读取当前凭据。服务地址与登录状态更新不会移动已固定的文件读取视图。

CLI 负责参数解析、显示、打开浏览器；通过 LoginOptions、Selection 与 LoginInteraction 传入必要信息，
不传 Invocation 或 flags map。登录算法与存储回归在本包；真实 argv、Server broker 与 kcfs 集成留在 CLI。
地址优先级、旧文件读取、原子私有保存与凭据隔离沿用 [CLI 合同](../../cli/README.md)。
本包不进入服务端执行、部署、模块 SDK 聚合或具体认证 provider。

地址选择、非秘密端点配置与私有路径推导共用 `internal/clientconfig`，使 Invocation 的旧公开配置入口
不依赖会话或 SDK；该共享包不含登录、凭据与服务端资源。
