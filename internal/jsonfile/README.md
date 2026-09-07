# internal/jsonfile/

小型 JSON 文件读写：独立临时文件、文件 fsync、rename 和父目录 fsync 后才确认替换成功。JSONL 追加保持每条记录完整并 fsync 后返回。调用方负责读改写互斥、对象验证和恢复语义；本包不定义协议对象或日志查询合同。
