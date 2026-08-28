# Changelog

## [v0.2.0] - 2026-08-28

### Added

- 飞书 WebHook 签名校验：支持 `secret` 与 `enable_signature` 配置，开启后对请求做 HMAC-SHA256 签名。
- `-s` flag to run in the foreground (console), which is also the default when no flag is given.
- `-d` flag to install and run LogMonitor as a background system service (Windows Service / systemd) via `kardianos/service`.
- `-c` flag 指定配置文件路径（原为 `-config`）。

## [v0.1.0] - 2026-08-28

### Added

- Monitor individual log files and log directories.
- Configurable log level regular expressions.
- Feishu WebHook notifications with context lines.
- Cross-platform release packages for Windows and Linux.
