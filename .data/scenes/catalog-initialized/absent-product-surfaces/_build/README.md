# absent-product-surfaces

这些入口必须不存在或明确拒绝：对象 LIST、`kc checkout` / snapshot-export、`kc connector-run`、`kc reconcile`、MCP 网关、APPEND/Stream 正路径、Agent 专用 `vfs-*` 工具。

构建与探：`TestRemovedCommandsAreRejected`、`TestAppendAndStreamSurfacesStayAbsent`。
