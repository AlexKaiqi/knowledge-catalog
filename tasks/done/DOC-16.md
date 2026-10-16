# DOC-16 · Provider 能力合同与跨 provider 等价性

- [x] 让 native Knowledge provider 进入共享合同套件，并建立跨 provider 等价性证据。所属阶段：M5 的基础纠偏，方向已选定，可独立推进。

**问题与影响：** 共享合同套件（`RepositoryContract` / `WriterContract`）与 provider 无关，但规模档选定的权威 provider `object/dolt` **从未接入**；一次性探针实测接入后 14 个子测试全绿、零跳过，说明缺口是覆盖矩阵而非实现。同时**跨 provider 等价性为空**，而 `SCALE_ARCHITECTURE.md` §5.3 把它列为迁移的核心差分测试。另有一处会 OOM 的静默全量回退路径（变化识别能力缺失时）所在包没有任何行为测试。

**处理范围：** 先接入合同套件，再建按步骤索引对齐的等价性 harness（不同 provider 的 commit 标识必然不同）；随后补能力拒绝毒化项与静默回退的行为测试。不改 ①/②/③ 公开语义；不新增第二套主题 owner。设计与被否决方案见[能力合同](../../doc/architecture.md)，门槛与分档见验证设计，执行序与完成定义归根本 TASK 的迁移记录。

**完成标准：** 等价性与毒化门槛按 `PV-01`…`PV-12` 实际执行并保存证据；`PAC-01`…`PAC-08` 在对应测试存在后才进入 `doc/engineering/architecture-invariants.md`；未执行或被跳过的档位不得计为通过；基线阻塞（`make check-validation` 的未解析引用、`check-surface` 缺 `rg`）先修或明确登记。

依据：[架构总览](../../doc/architecture.md)、验证设计、[规模设计](../../doc/engineering/testing.md) §5.3、[规模门槛](../../doc/engineering/testing.md) §7 P0。
