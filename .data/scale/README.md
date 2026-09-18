# Scale fixture

本目录只生成规模输入，不实现 KC 协议或新的 Write Surface。

独立压测用例见 [`CASES.md`](CASES.md)，所需环境和当前就绪性见
[`ENVIRONMENT.md`](ENVIRONMENT.md)。它们不属于 `.data/scenes` 功能验收，
也不由 `make test` 执行；总体模型和资格门槛仍以
[`docs/SCALE_BENCHMARK.md`](../../docs/SCALE_BENCHMARK.md) 为准。

```bash
python3 generator/generate.py --profile S0 --history H0 --out runs/s0-h0
python3 generator/generate.py --profile S3 --history H4 --events 10000 --out runs/s3-h4
python3 generator/generate.py --profile S5 --history H4 --events 10000 --out runs/s5-h4
```

输出 `manifest.json`、`model.json`、`bootstrap.ndjson`、`events.ndjson`。生成器流式写文件，S5 不把两百万张表放进内存；S5 的主体对象数为 106,000,000。load runner 应把每条 table family 翻译成现有 ChangeSet，再通过公开 Writer API 提交；逐条执行和判定遵循 `CASES.md`，结果证据遵循 `docs/SCALE_BENCHMARK.md`。
