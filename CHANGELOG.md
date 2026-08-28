# Changelog

## [v0.2.0] - 2026-08-28

### Added

- 飞书 WebHook 签名校验：支持 `secret` 与 `enable_signature` 配置，开启后对请求做 HMAC-SHA256 签名。
- `-s` flag to run in the foreground (console), which is also the default when no flag is given.
- `-d` flag to install and run LogMonitor as a background system service (Windows Service / systemd) via `kardianos/service`.
- `-c` flag 指定配置文件路径（原为 `-config`）。
- 飞书告警消息改为 interactive 卡片：头部颜色与表情按日志级别区分（ERROR/FATAL 🚨 红、WARN ⚠️ 橙、其余 ℹ️ 蓝），并通过字段展示来源、级别、文件、时间与日志上下文。

### Fixed

- 后台服务模式使用绝对配置路径并设置服务工作目录，避免 systemd 下相对路径 `conf/monitor.yml` 找不到文件。重跑 `-d` 会自动重装以修复已有安装。
- 修复去重表 `seen` 无界增长：`cooldown_seconds` 为 0 时不再写入，并定期清理已超冷却期的条目。
- 新增 `aggregate_seconds`：同来源+同级别的告警在窗口内合并为单条飞书消息，避免突发日志刷屏；默认 5 秒，0 表示不合并。
- 飞书卡片改用 `schema: "2.0"` 结构（`card.body.elements`），日志上下文以围栏代码块正确渲染；原 1.0 结构下代码块无法显示。

## [v0.1.0] - 2026-08-28

### Added

- Monitor individual log files and log directories.
- Configurable log level regular expressions.
- Feishu WebHook notifications with context lines.
- Cross-platform release packages for Windows and Linux.
