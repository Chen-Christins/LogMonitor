# LogMonitor

LogMonitor 是一个跨平台日志监控程序，支持监控单个日志文件或日志目录。日志级别命中配置后，程序会收集对应上下文，并通过飞书 WebHook 机器人发送告警。

## 部署

### 准备文件

将对应操作系统的可执行文件和配置模板放到同一目录：

```text
logmonitor/
├── LogMonitor              # Linux
├── LogMonitor.exe          # Windows
└── config.example.yaml
```

复制配置模板：

Linux：

```bash
cp config.example.yaml config.yaml
chmod +x LogMonitor
```

Windows PowerShell：

```powershell
Copy-Item config.example.yaml config.yaml
```

修改 `config.yaml` 后启动程序。

### 前台运行

Linux：

```bash
./LogMonitor -s -c config.yaml
```

Windows：

```powershell
.\LogMonitor.exe -s -c config.yaml
```

参数说明：

- `-c`：配置文件路径，默认是当前工作目录下的 `config.yaml`。
- `-s`：以前台模式运行，适合调试或由其他进程管理器托管。
- `-d`：安装并启动为系统服务，需要管理员或 root 权限。

### 后台服务

Linux：

```bash
sudo ./LogMonitor -d -c /etc/logmonitor/config.yaml
systemctl status LogMonitor
```

停止和移除服务：

```bash
sudo systemctl stop LogMonitor
sudo systemctl disable LogMonitor
```

Windows 请在管理员 PowerShell 中执行：

```powershell
.\LogMonitor.exe -d -c C:\LogMonitor\config.yaml
sc.exe query LogMonitor
```

停止和移除服务：

```powershell
sc.exe stop LogMonitor
sc.exe delete LogMonitor
```

## 完整配置

```yaml
log_sources:
  - name: linux-app
    file: "/var/log/myapp/app.log"
    level_regex: "\\[\\s*([A-Za-z][A-Za-z0-9_-]*)\\s*\\]"
    levels: [ERROR, FATAL]
    before_lines: 5
    after_lines: 5

  - name: application-directory
    directory: "/var/log/myapp"
    pattern: "*.log"
    recursive: true
    level_regex: "\\[\\s*([A-Za-z][A-Za-z0-9_-]*)\\s*\\]"
    levels: [WARN, ERROR, FATAL]
    before_lines: 10
    after_lines: 10

feishu:
  webhook_url: "${FEISHU_WEBHOOK_URL}"
  enable_signature: 1
  secret: "${FEISHU_SECRET}"

notification:
  cooldown_seconds: 60
  aggregate_seconds: 5
  max_context_lines: 100
  max_context_bytes: 65536
  retry_count: 3

runtime:
  memory_limit_mb: 256
  read_chunk_bytes: 65536
  max_line_bytes: 1048576
  max_tracked_files: 1000
```

## 日志源

`log_sources` 支持配置多个来源。每个来源必须在 `file` 和 `directory` 中选择一个，不能同时配置。

### 单个文件

```yaml
log_sources:
  - name: order-service
    file: "/var/log/order-service/app.log"
    level_regex: "\\[\\s*([A-Za-z][A-Za-z0-9_-]*)\\s*\\]"
    levels: [ERROR, FATAL]
    before_lines: 5
    after_lines: 5
```

### 日志目录

```yaml
log_sources:
  - name: order-service
    directory: "/var/log/order-service"
    pattern: "*.log"
    recursive: true
    level_regex: "\\[\\s*([A-Za-z][A-Za-z0-9_-]*)\\s*\\]"
    levels: [ERROR, FATAL]
    before_lines: 5
    after_lines: 5
```

字段说明：

- `name`：日志来源名称，会显示在飞书告警中。
- `file`：单个日志文件路径，支持 Linux 和 Windows 路径。
- `directory`：日志目录路径。
- `pattern`：目录内文件名匹配规则，默认 `*.log`。
- `recursive`：是否递归扫描子目录。
- `levels`：触发告警的日志级别，不区分大小写，默认 `[ERROR]`。
- `level_regex`：提取日志级别的正则表达式，必须且只能使用一个捕获组。
- `before_lines`：触发日志之前携带的上下文行数。
- `after_lines`：触发日志之后携带的上下文行数。

程序首次发现文件时从文件末尾开始读取，不发送已有历史日志。运行期间会处理新增文件、文件追加、文件截断和日志轮转。

### 日志级别正则

方括号级别日志：

```text
[2026-08-28 13:54:07.365826] 543705 tick_0 [ERROR] [root] request failed
```

```yaml
level_regex: "\\[\\s*([A-Za-z][A-Za-z0-9_-]*)\\s*\\]"
levels: [ERROR, FATAL]
```

普通文本日志：

```text
2026-08-28 13:54:07 ERROR request failed
```

```yaml
level_regex: "\\b(INFO|WARN|ERROR|FATAL)\\b"
levels: [ERROR, FATAL]
```

JSON 日志：

```json
{"time":"2026-08-28T13:54:07Z","level":"ERROR","message":"request failed"}
```

```yaml
level_regex: '"level"\s*:\s*"([^"]+)"'
levels: [ERROR]
```

## 飞书配置

```yaml
feishu:
  webhook_url: "${FEISHU_WEBHOOK_URL}"
  enable_signature: 1
  secret: "${FEISHU_SECRET}"
```

- `webhook_url`：飞书自定义机器人 WebHook 地址，必填。
- `enable_signature`：签名开关，支持 `1`、`0`、`true`、`false`。
- `secret`：飞书机器人签名密钥。启用签名时必填。

未配置 `enable_signature` 时，程序根据 `secret` 是否为空自动决定是否签名。显式设置为 `0` 或 `false` 时，即使存在 `secret` 也不会签名。

配置文件支持 `${ENV_NAME}` 环境变量。推荐通过环境变量保存 WebHook 和密钥，避免把敏感信息直接写入配置文件。

Linux：

```bash
export FEISHU_WEBHOOK_URL='https://open.feishu.cn/open-apis/bot/v2/hook/...'
export FEISHU_SECRET='...'
./LogMonitor -s -c config.yaml
```

Windows PowerShell：

```powershell
$env:FEISHU_WEBHOOK_URL = "https://open.feishu.cn/open-apis/bot/v2/hook/..."
$env:FEISHU_SECRET = "..."
.\LogMonitor.exe -s -c config.yaml
```

## 告警配置

```yaml
notification:
  cooldown_seconds: 60
  aggregate_seconds: 5
  max_context_lines: 100
  max_context_bytes: 65536
  retry_count: 3
```

- `cooldown_seconds`：相同文件和相同触发日志的去重时间，`0` 表示不去重。
- `aggregate_seconds`：相同来源、相同级别告警的合并窗口，默认 `5` 秒，`0` 表示不合并。
- `max_context_lines`：单条告警或聚合消息最多保留的日志行数，默认 `100`。
- `max_context_bytes`：单条告警或聚合消息最多保留的日志字节数，默认 `65536`。
- `retry_count`：飞书发送失败时的总尝试次数，默认 `3`。

飞书告警使用卡片消息，包含来源、级别、文件路径、时间和日志上下文。

## 内存保护

```yaml
runtime:
  memory_limit_mb: 256
  read_chunk_bytes: 65536
  max_line_bytes: 1048576
  max_tracked_files: 1000
```

- `memory_limit_mb`：Go 运行时内存软上限，默认 `256 MB`。
- `read_chunk_bytes`：日志分块读取大小，默认 `64 KB`，不会一次加载全部新增日志。
- `max_line_bytes`：单行日志最大字节数，默认 `1 MB`，超长行会被丢弃。
- `max_tracked_files`：同时打开并跟踪的文件数量上限，默认 `1000`。

`memory_limit_mb` 是 Go 运行时软限制。Linux 生产环境建议通过 systemd 增加操作系统级限制：

```ini
[Service]
MemoryMax=256M
Restart=always
RestartSec=5
```

运行服务的用户还需要具备日志文件和日志目录的读取权限。
