# Scale fixture

本目录只生成规模输入与压测用例规格，不实现 KC 协议或新的 Write Surface。

- **压测用例**按场景树组织在 [`scenes/`](scenes/README.md)：状态树 + construct +
  探针，入口与运行合同见 [`CASES.md`](CASES.md)，环境配置合同见
  [`ENVIRONMENT.md`](ENVIRONMENT.md)。树视角是性能测试：一条探针回答一个测量
  问题，量级阶梯出曲线。它们不属于 `.data/scenes` 功能验收，也不由 `make test`
  执行；总体模型和资格门槛仍以 [`docs/SCALE_BENCHMARK.md`](../../docs/SCALE_BENCHMARK.md)
  为准（其 §10 数值门槛重登记前只作默认参考）。
- 执行器**不启动容器**：只读取环境配置（[`scenes/env.example.yaml`](scenes/env.example.yaml)
  形状）并绑定一个已部署、已就绪的 KC 环境；runner 能力词汇表见
  [`scenes/README.md`](scenes/README.md) §4。完整 load runner（到达模型、长窗口、
  故障注入）尚未实现；已有 `runner/smoke.py` 冒烟 runner，覆盖其中 client-timer
  可测的探针子集，用途与边界见 [`runner/README.md`](runner/README.md)。
- 指标唯一登记表：[`scenes/metrics.yaml`](scenes/metrics.yaml)。

```bash
python3 .data/scale/scenes/perf_tree.py --check        # 用例树结构检查
python3 .data/scale/scenes/perf_tree.py --env my-env.yaml   # 环境配置形状校验

python3 generator/generate.py --profile S0 --history H0 --out runs/s0-h0
python3 generator/generate.py --profile S3 --history H4 --events 10000 --out runs/s3-h4
python3 generator/generate.py --profile S5 --history H4 --events 10000 --out runs/s5-h4
```

输出 `manifest.json`、`model.json`、`bootstrap.ndjson`、`events.ndjson`。生成器流式写文件，S5 不把两百万张表放进内存；S5 的主体对象数为 106,000,000。load runner 应把每条 table family 翻译成现有 ChangeSet，再通过公开 Writer API 提交；逐条执行和判定遵循 `CASES.md`，结果证据遵循 `docs/SCALE_BENCHMARK.md`。
