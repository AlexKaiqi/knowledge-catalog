# 架构图资产

- `core-responsibilities.svg`：分层职责与扩展边界主图。
- `agent-use-loop.svg`：Agent 从发现、使用到修订的责任交接辅助图。
- 同名 PNG 是两倍分辨率的展示副本；SVG 是可编辑源。

图表达设计职责与协作，不作为精确 import、部署拓扑或实施验收图。
后续插件和外部授权申请目标用虚线表达；现有 `controlplane` 仅表示按需采用的应用流程。

语义沿 [Core Architecture](../../reviewed/core-architecture.md)、
[Object System](../../reviewed/object-system.md)、
[Plugin Extension](../../reviewed/plugin-extension.md)、
[Service](../../reviewed/service.md) 与
[Permissions](../../reviewed/permissions.md) 的主题所有权维护，不在资产目录另立合同。

SVG 使用命名 `<g id="…">` 组织模块，文字、块和连线均可直接编辑；共用样式在 SVG 内。
更改内容后同步渲染 PNG，并检查整图文字、边界与箭头。

本机 macOS 可复用已有 Quick Look 与 `sips`，只需 Python 标准库，不安装依赖：

```sh
python3 docs/assets/architecture/render_png.py
```

也可指定单个 SVG。脚本只改变 PNG；临时的方形渲染画布不会写回源文件。
