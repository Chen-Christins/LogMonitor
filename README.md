# LogMonitor

跨平台 Go 日志监控程序，支持监控单个文件或日志目录。命中配置的日志级别后，将日志上下文发送到飞书 WebHook 机器人。

## 项目结构

```text
LogMonitor/
├── bin/
│   └── LogMonitor.exe       # 本地构建产物（不提交）
├── .github/
│   └── workflows/
│       ├── ci.yml            # master 分支测试和检查
│       └── release.yml       # Tag 发布跨平台程序包
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
├── CHANGELOG.md           # 版本记录，发布流水线从这里读取版本
├── go.mod
└── README.md
```

## 使用

```bash
go run ./cmd/logmonitor -c config.yaml
```

运行模式：

- `-s`：前台运行（终端阻塞，Ctrl+C 退出）。不传任何标志时默认就是前台模式。
- `-d`：以后台系统服务方式安装并启动（Windows Service / systemd），安装需要管理员/root 权限。启动后原进程退出，监控由系统服务进程在后台持续运行。

```bash
./LogMonitor -s -c config.yaml   # 前台
sudo ./LogMonitor -d -c config.yaml  # 后台（Linux 需 root 安装服务）
```

后台服务的停止与卸载使用系统命令：

- Linux：`systemctl stop LogMonitor`、`systemctl disable LogMonitor`
- Windows：`sc stop LogMonitor`、`sc delete LogMonitor`

复制 `config.example.yaml` 后填写飞书 WebHook。`file` 与 `directory` 必须二选一；目录使用 `pattern` 匹配文件名，并可用 `recursive` 递归子目录。Windows 和 Linux 路径都可配置，程序应在对应操作系统上运行。

程序首次发现文件时从文件末尾开始读取，避免发送历史告警。之后每秒检查追加内容和目录中新文件，并处理文件截断。

## 配置说明

- `levels`：大小写不敏感的日志级别关键字，默认是 `ERROR`。
- `level_regex`：日志级别提取正则，必须包含一个捕获组；默认匹配 `[INFO]`、`[WARN]`、`[ERROR]` 等格式。
- `before_lines`、`after_lines`：告警前后的上下文行数。
- `cooldown_seconds`：相同文件、相同触发行的去重时间，`0` 表示不去重。
- `max_context_lines`：单条消息最多携带的日志行数。
- `retry_count`：飞书发送失败后的总尝试次数。
- `secret`：飞书机器人开启签名校验时填写，未开启则留空。
- `enable_signature`：是否启用飞书签名，支持 `1`、`0`、`true`、`false`。启用时必须配置 `secret`；未配置该字段时，会根据 `secret` 是否为空自动判断。

日志级别通过每个日志源的 `level_regex` 提取。正则必须包含一个捕获组，捕获组内容会与 `levels` 比较。对于包含方括号级别的日志，程序只匹配级别字段，不会因为日志正文中出现 `ERROR` 而误报。

告警以飞书 interactive 卡片消息发送：头部颜色与表情图标按日志级别区分（`ERROR`/`FATAL` 为 🚨 红色，`WARN` 为 ⚠️ 橙色，其余为 ℹ️ 蓝色），卡片内展示来源、级别、文件、时间与日志上下文代码块。

`notification.aggregate_seconds` 控制合并窗口：同来源+同级别的告警在窗口内会被合并成一条消息（标题显示条数），避免突发日志刷屏；默认 `5` 秒，设为 `0` 则每条独立发送。

例如你的日志格式可以使用默认配置：

```yaml
level_regex: "\\[\\s*([A-Za-z][A-Za-z0-9_-]*)\\s*\\]"
levels: [ERROR, FATAL]
```

如果日志格式是 `2026-08-28 ERROR message`，可以配置：

```yaml
level_regex: "\\b(INFO|WARN|ERROR|FATAL)\\b"
levels: [ERROR, FATAL]
```

如果级别位于 JSON 字段中，可以配置：

```yaml
level_regex: '"level"\\s*:\\s*"([^"]+)"'
levels: [ERROR]
```

配置支持 `${ENV_NAME}` 环境变量。推荐把 WebHook 和密钥放在环境变量中：

```powershell
$env:FEISHU_WEBHOOK_URL = "https://open.feishu.cn/open-apis/bot/v2/hook/..."
$env:FEISHU_SECRET = "..."
./LogMonitor.exe -c config.yaml
```

Linux 示例：

```bash
export FEISHU_WEBHOOK_URL='https://open.feishu.cn/open-apis/bot/v2/hook/...'
export FEISHU_SECRET='...'
./LogMonitor -c config.yaml
```

构建当前平台版本：

```bash
go build -o bin/LogMonitor ./cmd/logmonitor
```

## 下载部署

推送到 `master` 时，GitHub Actions 会读取 `CHANGELOG.md` 第一条版本标题，并自动创建 Draft Release。版本标题格式：

```markdown
## [v0.1.0] - 2026-08-28
```

版本标题放在 Markdown 代码块中也可以识别，例如：

```markdown
## [v0.1.0] - 2026-08-28
```

代码块标记不会影响版本提取，流水线只匹配其中的 `##` 版本标题。无论使用 `[v0.1.0]` 还是 `v0.1.0` 格式都支持。

推送代码即可触发：

```bash
git push origin master
```

该版本对应的 Changelog 内容会作为 Release Notes。也可以在 GitHub Actions 页面手动运行 `Release`，通过 `version` 输入框指定版本，例如 `v0.1.1`。如果该版本 Release 已存在，流水线会跳过，避免重复发布。

Release 中会提供以下程序包：

- `LogMonitor-vX.Y.Z-windows-amd64.tar.gz`
- `LogMonitor-vX.Y.Z-linux-amd64.tar.gz`
- `LogMonitor-vX.Y.Z-linux-arm64.tar.gz`
- `SHA256SUMS`

每个程序包包含可执行文件、`config.example.yaml` 和 `README.md`。下载后解压，并将配置模板复制成实际配置：

```bash
cp config.example.yaml config.yaml
```

Windows PowerShell：

```powershell
Copy-Item config.example.yaml config.yaml
```

然后修改 `config.yaml` 中的日志路径和飞书配置，启动程序即可。真实的 `config.yaml` 已被 `.gitignore` 忽略，不会被提交。
