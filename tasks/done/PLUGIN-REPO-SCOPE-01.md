# PLUGIN-REPO-SCOPE-01 · Repo 配置与写读共同作用域（2026-10-13）

- [x] 文档调整完成：插件由 Repo 配置并以该 Repo 为作用域；写入后的派生维护、直接读取和 Dataset 来源消费使用该仓配置的贡献。Server 负责装配执行，Dataset 只组合；首批假设插件贡献没有冲突，不设计优先级、覆盖或冲突裁决。
- 范围：插件扩展 owner，索引/LiveAccess、CLI、术语、重构计划及设计索引的边界同步；只调整文档并重读文档图，不实施插件 API、配置存储或路由，不改 Go、不提交。
- 完成条件：写入、查询和访问过程能定位来源 Repo、取得配置与贡献、传递受限 payload 并采用结果；独立复核、owner/图重读、`make check-docs`、`make quality` 及增量空白检查通过后只记文档完成。
- 质量处置：原文混用“Repository/对象采用”、Server 默认选择及固定来源插件版本，判定为插件配置归属和知识声明混淆；本轮统一为 Repo 配置，Object 保存 Schema/access/Binding，运行引用和凭据由 Server 持有。插件冲突暂不纳入本轮设计分母；真实需求出现时回到该 owner 选定，不以假设无冲突放宽已有授权、查询支持或完整性合同。
- 验证：独立增量复核通过，CLI U8 遗留的“无采用声明”已改为来源 Repo 未配置；已重读变更 owner 与文档图。`make check-docs`（33 文档／174 关系）、`make quality`（退出 0，日志 `/tmp/kc-repo-plugin-scope-quality-final-20261013.log`）及文档空白检查通过。仅确认文档调整，Repo 配置/登记和真实路由仍待实施；共享工作树中的实施顺序、LiveAccess U20 等并发增量保留，不计本轮交付。
