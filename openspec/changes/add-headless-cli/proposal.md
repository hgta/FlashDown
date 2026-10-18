# Proposal: add-headless-cli

## Why

这款软件的初衷是让旧机器也有一个轻量下载器，但 v0.2.1 在 Windows 8.1 上只能弹一句"不受支持"。

8.1 实机验证给出了关键结论：程序**能启动**、版本探测正确（`6.3.9600`）、日志正常写入，唯一失败的是 WebView2 窗口。
而下载内核 `internal/engine` 与界面完全解耦，只依赖 `internal/config` 和 `golang.org/x/sys/windows`。

也就是说，挡在 8.1 面前的不是"下载能力"，只是"窗口"。既然这样，就应该给旧机器一条没有窗口的路。

## What Changes

- **新增** 独立的命令行程序 `flashdown-cli.exe`：不 import Wails，复用现有引擎（分块、多连接、断点续传、重定向、重试全部保留）
- **新增** 参数：URL、`-o` 目录、`-n` 文件名、`-j` 连接数、`-q` 静默
- **新增** 终端进度：TTY 下单行刷新（百分比 / 速度 / 剩余 / 已完成），重定向到文件时改为每 10% 一行
- **新增** Ctrl+C 语义：暂停并落盘 journal，退出码 130；再次运行同一条命令自动续传
- **修改** `internal/startup`：增加 GUI / CLI 两种模式。CLI 不检查 WebView2，版本低于 10.0.17763 只记警告、不拦截
- **修改** GUI 的拒绝文案：在旧系统上直接告诉用户「可以用 `flashdown-cli.exe <链接>`」，而不是只说不支持
- **修改** 发布产物：zip 内同时包含 `flashdown.exe` 与 `flashdown-cli.exe`
- **不做**：不支持 Windows 7（未验证）；不为 8.1 提供图形界面（WebView2 已停止支持，另起 Win32 界面不属于本变更）

## Capabilities

### New Capabilities

- `headless-cli`: 无界面下载入口 —— 参数、平台降级策略、进度输出、中断与续传、退出码、以及发布形态

### Modified Capabilities

- `startup-diagnostics`: 启动检查从「一律拦截」变为「按运行模式决定严格程度」，并新增旧系统引导命令行模式的文案

## Impact

- **代码**：
  - 新增 `cmd/flashdown-cli/`（main 包）与 `internal/cli/`（可测试的编排逻辑）
  - 新增 `internal/version/`：版本常量从 `app` 包移出，供 GUI 与 CLI 共用（CLI 不能 import `app`，否则会把 Wails 拉进依赖）
  - `internal/startup`：`Mode` 与 `CheckFor`
  - `main.go`：显式使用 `ModeGUI`
- **发布**：CI 增加一步 `go build`，zip 从 2 个文件变成 3 个
- **文档**：README、release notes、`SYSTEM-REQUIREMENTS.txt` 说明两种入口
- **风险**：Windows 8.1 仍不在 Go 的官方支持范围内，CLI 只能标注为"尽力支持"
