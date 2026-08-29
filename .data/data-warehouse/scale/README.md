# Data warehouse scale fixture

本目录只生成墙外数仓 provider 的规模输入，不实现 KC 协议或新的 Write Surface。

```bash
../../../.venv/bin/python generator/generate.py --profile S0 --history H0 --out ../runs/scale/s0-h0
../../../.venv/bin/python generator/generate.py --profile S3 --history H4 --events 10000 --out ../runs/scale/s3-h4
../../../.venv/bin/python generator/generate.py --profile S5 --history H4 --events 10000 --out ../runs/scale/s5-h4
```

输出 `manifest.json`、`model.json`、`bootstrap.ndjson`、`events.ndjson`。生成器流式写文件，S5 不把两百万张表放进内存；S5 的主体对象数为 106,000,000。load runner 应把每条 table family 翻译成现有 ChangeSet，再通过公开 Writer API 提交；结果证据遵循 `docs/SCALE_BENCHMARK.md`。
