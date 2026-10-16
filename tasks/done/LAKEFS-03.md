# LAKEFS-03 · ChangeSet 传输压缩与差分预检并发化

- [x] 本轮完成：typed Client 对 >512 KiB 请求体自动 gzip（`Content-Encoding: gzip`，标准库），服务面按头透明解压且上限对解压后字节强制（gzip 炸弹不放大）；writer commit 路由独立体量上限（默认 512 MiB，`KC_MAX_COMMIT_REQUEST_BYTES` 覆盖），其余服务路由维持 8 MiB；`remoteCurrentDigests` 差分预检改有界并发（16，首错误中止），语义与串行一致。红证据（大请求体无压缩头、gzip 体被当明文拒、预检串行 max in-flight=1）先行复现后转绿；`client` 整包、`snapshot/lakefs` 整包、`cli` 定向批（解码/gzip/上限/预检/recovery/-race）与 `make check-docs` 通过。`cli` 整包本轮无法认证：并行 DATASET-03 工作线的未提交 WIP 自带 3 个失败用例与 2 条待补证据的 `/ui` 路由（route-count 守卫 86→88），且 Dolt 引擎夹具在与其会话并发时启动挂起（`snapshot/dolt/session.go` stdout 读取 IO wait）——均不在本条改动面，待并行线收口后随全量套件认证。

**Goal：** typed Client 对大请求体透明 gzip（`Content-Encoding: gzip`，仅标准库，不新增依赖）；服务面按头透明解压，且上限对解压后字节强制（gzip 炸弹不放大）；writer commit 路由独立大体量上限（默认 512 MiB，`KC_MAX_COMMIT_REQUEST_BYTES` 覆盖），其余服务路由维持 8 MiB；`remoteCurrentDigests` 差分预检改有界并发，结果与错误语义同串行逐操作一致。

**Non-Goals：** 不新增 HTTP 路由；不改 ChangeSet/digest 合同与 CLI argv；不引入 zstd 等未批准依赖；不动非 typed 路径；不宣称端到端部署提速数字（归 SCALE-01）。

**不变量：** `A-01`、`V-01`、`W-02`；请求体上限对解压后字节生效；预检结果（digest map）与串行语义一致——unresolved/空 digest 仍跳过、非 unresolved 错误仍整批失败；预检只读，不写 authority。

**选定与否决：** 选定客户端 >512 KiB 请求体自动 gzip + 服务面按 `Content-Encoding: gzip` 解压（`compress/gzip`，LimitReader 作用于解压流）；writer commit 上限默认 512 MiB（实测 228 MB 约两倍余量），其余路由维持 8 MiB；预检并发默认 16、首错误中止。否决 zstd（依赖未批准）；否决为预检新增批量 resolve 路由（闭合面不扩张，留待真实测量证明需要）；否决「首导入跳过预检」启发式。

**接口与验收：** 公开形状不变。先运行会失败的证据：大请求体不带 `Content-Encoding: gzip`、gzip 请求体被 8 MiB 明文限制误拒、预检串行（max in-flight=1）；再实现；以 `client`、`cli` 定向回归与 `make check-docs` 验收，不以 skip 换绿。
