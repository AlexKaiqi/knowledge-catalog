# LIVEACCESS-CLI-DOC-01 · LiveAccess 的 CLI、契约与承诺（2026-10-13）

- [x] 历史核对完成，所有权判断已撤回：真实 help 只证明四入口存在，不能把对象/声明检查/审计调用方入口视作 LiveAccess 的能力；错列的表由 LIVEACCESS-EFFECT-BOUNDARY-01 删除，改写插件对既有读取的效果。原合并的契约与承诺保留。
- 验证：四个真实 `kc help` 均正常返回，已与公开命令表、SURFACE、LiveAccess 与审计查询合同核对；没有将 `--request-id` 当历史查询条件。18 用例原 ID/标题不变，可见正文 996 字符，owner/图已重读；文档图、产品关联、增量空白及 `make quality`（退出 0，日志 `/tmp/kc-liveaccess-cli-doc-quality-20261013.log`）通过。本轮仅改该篇及本条，不改运行实现。
