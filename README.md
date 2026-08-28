# LogMonitor

跨平台 Go 日志监控程序，支持监控单个文件或日志目录。命中配置的日志级别后，将日志上下文发送到飞书 WebHook 机器人。

## 项目结构

```text
LogMonitor/
├── bin/
│   └── LogMonitor.exe       # Windows 构建产物
├── cmd/
│   └── logmonitor/
│       └── main.go          # 程序入口
├── internal/
│   ├── config/
│   │   ├── config.go        # YAML 配置和校验
│   │   └── config_test.go
│   ├── feishu/
│   │   └── client.go        # 飞书 WebHook 客户端
│   └── monitor/
│       ├── monitor.go       # 文件跟踪、目录发现、上下文收集
│       └── monitor_test.go
├── config.example.yaml
├── go.mod
└── README.md
```

## 使用

```bash
go run ./cmd/logmonitor -config config.yaml
```

复制 `config.example.yaml` 后填写飞书 WebHook。`file` 与 `directory` 必须二选一；目录使用 `pattern` 匹配文件名，并可用 `recursive` 递归子目录。Windows 和 Linux 路径都可配置，程序应在对应操作系统上运行。

程序首次发现文件时从文件末尾开始读取，避免发送历史告警。之后每秒检查追加内容和目录中新文件，并处理文件截断。

## 配置说明

- `levels`：大小写不敏感的日志级别关键字，默认是 `ERROR`。
- `before_lines`、`after_lines`：告警前后的上下文行数。
- `cooldown_seconds`：相同文件、相同触发行的去重时间，`0` 表示不去重。
- `max_context_lines`：单条消息最多携带的日志行数。
- `retry_count`：飞书发送失败后的总尝试次数。
- `secret`：飞书机器人开启签名校验时填写，未开启则留空。

配置支持 `${ENV_NAME}` 环境变量。推荐把 WebHook 和密钥放在环境变量中：

```powershell
$env:FEISHU_WEBHOOK_URL = "https://open.feishu.cn/open-apis/bot/v2/hook/..."
$env:FEISHU_SECRET = "..."
./LogMonitor.exe -config config.yaml
```

Linux 示例：

```bash
export FEISHU_WEBHOOK_URL='https://open.feishu.cn/open-apis/bot/v2/hook/...'
export FEISHU_SECRET='...'
./LogMonitor -config config.yaml
```

构建当前平台版本：

```bash
go build -o bin/LogMonitor ./cmd/logmonitor
```
