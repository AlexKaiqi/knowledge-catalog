# INDEX-CATCHUP-01 · 看见索引正在追赶的这一趟

## 目标

维护者能读到服务当前这一趟索引构建的进度，且这个进度与显式准备看到的是同一份数。

## 关闭

2026-10-22 并入 `INDEX-PUBLISH-DECOUPLE-01`，未按本文件单独验收。

并入理由：进度可见不是独立需求，而是解耦的必然产物。同步构建时"发布返回"就是"构建完成"，没有可以
观察的窗口；一旦发布先接受、索引后台追，`indexes:status` 就是这个窗口的唯一出口。两者是同一次改动的
两面，分成两条会让进度口的宿主（构建调度）和它的消费者（发布路径）落在两个文件里。

承接内容与变化：

- 原验收八条（无追赶时 `kc index` 的 stdout 恰为一行退出 0、`done` 单调递增且 `total` 同趟稳定、
  `total` 未知只打印 `repo  done`、成功或失败后 status 回落 `{}`、未配置索引实例返回
  `CAPABILITY_UNSATISFIED`、`kc index prepare` 进行中行与 status 同 `repository`、非 TTY 逐次追加、
  进程重启从 0 重计）逐条保留在 `INDEX-PUBLISH-DECOUPLE-01` 的验收第 8–12 条。
- 原"口径"一段（`done`/`total` 按对象计、含被声明排除对象、构建结束 `done == total` 而 `objectCount`
  更小、两者不互相印证）保留为 `INDEX-PUBLISH-DECOUPLE-01` 的设计决定 5 与验收第 14 条。
- 原"不做"里"不把 `indexes:prepare` 改成异步作业来换取进度"一条的表述需要更新：解耦之后
  `indexes:prepare` 的对外契约不变（同步返回已就绪结果），但发布路径不再经它。该约束以
  "`indexes:prepare` 保持同步语义"的形式保留在 `INDEX-PUBLISH-DECOUPLE-01` 的"不做"里。
- 原"前置 `INDEX-SURFACE-01` 先落命名"保留：`indexes:status` 与 `kc index` 的进度输出依赖该任务已定的
  命令家族与路由命名。
