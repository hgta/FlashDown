# Proposal: add-startup-diagnostics

## Why

v0.1.0 在 Windows 8.1 x64 上双击 `flashdown.exe` 后**完全没有任何反应**，也没有任何错误提示。

原因链条已经查清：`-H windowsgui` 编译的进程没有控制台，`main.go` 用 `log.Fatal` 报告 `wailsApp.Run()` 的失败，
stderr 被丢弃，进程以非零码静默退出。用户拿不到任何线索，只能来问"是不是不兼容"。

更深的问题不是"Win8.1 不受支持"（那是既定事实：Go 1.21+ 与 WebView2 都以 Win10 为下限），
而是**任何启动期失败都是无声的**——缺 WebView2 的 Win10 LTSC、被安全软件拦截、配置损坏，表现都一样：
窗口不出现，进程消失。这一版必须让失败自己开口说话。

## What Changes

- **新增启动前置检查**（早于 `application.New`，因此早于任何 WebView2 调用）：
  - Windows 版本：用 `RtlGetVersion` 读取真实版本（不读注册表 `CurrentMajorVersionNumber`，该值 Win10 才有），
    低于 `10.0.17763` 即判定不支持
  - WebView2 运行时：读注册表 `HKLM\SOFTWARE\WOW6432Node\Microsoft\EdgeUpdate\Clients\{F3017226-FE2A-4295-8BDF-00C3A9A7E4C5}` 的 `pv`
- **新增可见失败**：上述检查不通过时弹 Windows MessageBox，写明"需要 Windows 10 1809+（64 位）"或
  "缺少 WebView2"，并给出当前系统版本；WebView2 缺失时"确定"按钮打开微软下载页
- **新增启动日志**：每次启动失败写 `%LOCALAPPDATA%\FlashDown\startup.log`（系统版本、WebView2 检测值、
  `Run()` 的原始错误、时间戳），弹窗中给出该路径
- **修改** `Run()` 失败的处理：由 `log.Fatal` 改为"写日志 + 弹窗 + 非零退出码"，stderr 有控制台时仍输出
- **新增发布侧声明**：README、release 说明、zip 内的 `系统要求.txt` 写明 Windows 10 1809+ / 64 位 / WebView2
- **不做**：不支持 Windows 7 / 8 / 8.1。Go 1.21+ 与 Wails v3 均以 Win10 为下限，WebView2 对 8.1 的支持
  止于 Edge 109（2024-01），这不是靠适配能补上的缺口
- **不做**：32 位（386）构建。没有证据表明存在 32 位用户，且会牵动 syso、CI 与 WebView2 x86 三条线

## Capabilities

### New Capabilities

- `startup-diagnostics`: 启动前置检查（系统版本、WebView2）、失败可见化（MessageBox）、启动日志、以及对应的系统要求声明

### Modified Capabilities

（无 —— 现有 `main-window-ui`、`task-management` 等能力的行为不变，本变更只作用于进程启动阶段）

## Impact

- **代码**：
  - `main.go`：在 `config.Load()` 之前插入检查；`Run()` 错误处理改为可见
  - 新增 `internal/startup`（或 `app/startup.go`）：版本检测、WebView2 检测、弹窗、日志；不依赖 engine，可单测
  - 仅使用 `golang.org/x/sys/windows`（已在依赖树中），不引入新模块
- **构建**：无变化，仍为 `windows/amd64` + `-H windowsgui`
- **发布**：`RELEASE_NOTES.md`、`README.md`、CI 打包步骤（zip 内追加 `系统要求.txt`）
- **平台**：明确下限 Windows 10 1809（build 17763）/ Windows 11，x64
- **风险**：检查本身若写错，会把能跑的机器挡在门外（例如企业环境 WebView2 装在非默认位置）。
  因此检测必须"宽进严出"：读不到 ≠ 不存在，只有拿到明确的低版本或明确的缺失证据才拦截
