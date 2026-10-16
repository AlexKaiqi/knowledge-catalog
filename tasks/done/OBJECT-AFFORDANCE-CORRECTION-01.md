# OBJECT-AFFORDANCE-CORRECTION-01 · 按实际任务纠正 Object 能力清单（2026-10-13）

- [x] 文档纠正完成：Guideline、CLI owner 与计划已将 Schema 列举／描述归入 Object 基本能力，目录预览／提交与命令回执按 CLI 任务说明；目标基础读取直接交付固定版本及维护内容摘要，不要求先运行独立 resolve。
- 本轮只改上述文档及本条，不改 Go、现有命令／API、场景或运行配置，不提交。核对前已有统一 URN／locator 文档改动，保留其目标和在途修改；独立复核、owner／图重读、`make check-docs`、`make quality` 与本轮增量检查通过后只标记文档完成。
- 实施差距：当前 `cli/product_encode.go` 的 read 输出未交付维护摘要，`cli/desired.go` 的目录预览／提交通过 Resolve 接口取摘要，维护用例也通过独立命令取得更新条件。统一读取入口实施时补同依据的维护信息及真实“读取 → 更新”回归，再处理旧诊断入口；不将移出基础清单写成已删除命令或内部能力。
- 验证：独立增量复核通过，已重读变更 owner 及文档图；`make check-docs`（33 文档／172 关系）、`make quality`（退出 0，日志 `/tmp/kc-object-affordance-correction-quality-20261013.log`）与本轮空白检查通过。只完成文档纠正，未实现新的读取输出或删除现有 CLI；同文件中其它插件增量属于并发工作，不计入本轮交付。
