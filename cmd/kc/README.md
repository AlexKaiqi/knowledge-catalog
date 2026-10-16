# cmd/kc/

最薄可执行入口：只把进程参数交给 `cli` facade。命令表、解析、装配和业务流程都不放在这里。

本机构建和客户端安装包见 [`scripts/package/README.md`](../../scripts/package/README.md)。
客户端与 `serve` 沿用同一个 `kc` 二进制。
