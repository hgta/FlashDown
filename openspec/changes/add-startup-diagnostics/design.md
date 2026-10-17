## Context

v0.1.0 的启动路径：

```
main()
  ├─ config.Load()            // 失败 → log.Fatalf
  ├─ engine.New / Restore
  ├─ application.New(...)     // WebView2 在这里被触碰
  └─ wailsApp.Run()           // 失败 → log.Fatal(runErr)
```

`-H windowsgui` 的进程没有控制台，`log.Fatal` 写 stderr → 丢弃 → `os.Exit(1)`。
用户侧唯一可观测的现象是"双击没反应"。

Windows 8.1 x64 实测：无任何弹窗。这一点很有价值——如果 PE 导入表引用了 Win8.1 不存在的函数，
加载器会弹"无法定位程序输入点"。没有弹窗说明进程成功进入 `main`，随后静默退出。
也就是说，**在 Windows 层面加检测是有效的**，不需要外部启动器。

另外两个事实约束了方案空间：

1. Go 1.21 起最低支持 Windows 10；本项目是 1.25。
2. Wails v3 依赖 WebView2，而 WebView2 对 Win7/8.1 的支持止于 Edge 109（2024-01 停止）。

因此本设计只解决"失败要可见、要可诊断"，不解决"让 Win8.1 能跑"。

## Goals / Non-Goals

**Goals:**

- 任何启动期失败都以模态对话框呈现，包含原因、当前系统版本、日志路径
- 每次失败都留下 `startup.log`，让不会查事件查看器的用户只需发一个文件
- 检测逻辑可单测：不依赖真实注册表/真实 OS
- 检测不得误伤能正常工作的机器（尤其是缺注册表项但 WebView2 实际可用的环境）

**Non-Goals:**

- 不支持 Windows 7 / 8 / 8.1（技术不可行，见 Context）
- 不做 32 位（386）构建
- 不做 NSIS 安装包与安装期版本检查（可作为后续 change）
- 不改变主窗口运行时行为，不引入崩溃上报

## Decisions

### D1. 检测放在 `internal/startup`，以纯函数 + 注入实现

新增包，暴露最小接口：

```go
type Info struct {
    OSVersion      string   // "6.3.9600"
    OSVersionKnown bool
    WebView2       string   // "120.0.2210.91" 或 ""
}

// Check 只做判断，不做 IO 之外的副作用。
func Check(readVersion func() (major, minor, build int, err error),
           readWebView func() string) (Info, error)
```

`main.go` 只在两处调用：`startup.Check` 与 `startup.Fail`。这样：
- 判断逻辑可以用表驱动测试覆盖
- `main` 仍然只负责装配（现有风格）

### D2. 版本号用 `RtlGetVersion`，不用注册表、不用 `GetVersionEx`

- `GetVersionEx` 会被 manifest 的兼容性声明欺骗（Win8.1 上可能报告 6.2）。
- 注册表 `CurrentMajorVersionNumber` **只在 Windows 10 存在**；而 Wails 自带的
  `internal/operatingsystem.GetWindowsVersionInfo` 正是读它，在 8.1 上会得到 `-1.-1.9600`。
- `RtlGetVersion`（ntdll）返回真实的 `RTL_OSVERSIONINFOW`，不受 manifest 影响。

读取失败时：`OSVersionKnown=false`，**放行**，只在日志里记录。理由：读不到版本不等于系统太旧，
而"拦错人"的代价（能用的机器被挡住）高于"漏放一次"。

### D3. WebView2 检测三源，读不到注册表再看磁盘

顺序：

```
1) HKLM\SOFTWARE\Microsoft\EdgeUpdate\Clients\{F3017226-...}\pv
2) HKLM\SOFTWARE\WOW6432Node\Microsoft\EdgeUpdate\Clients\{F3017226-...}\pv
3) %ProgramFiles(x86)%\Microsoft\EdgeWebView\Application\<version>\
```

三条都不成立才判定缺失。WebView2 的 Evergreen 安装器一定会写注册表，但企业镜像/绿色部署
可能只留下磁盘目录，第 3 条就是给这种情况兜底。

与版本检测不同，这里**读不到就判缺失**是安全的：WebView2 确实不存在时，Wails 必然失败，
提前拦截只是把"无反应"换成"说清楚"。

### D4. 弹窗用 `user32.MessageBoxW`，链接用 `rundll32`

- 只需要一个"确定"，不需要自定义按钮，`MessageBoxW` 足够；避免引入 TaskDialog 与 COM。
- 图标 `MB_ICONERROR`，按钮 `MB_OKCANCEL`：确定 → 打开 WebView2 下载页，取消 → 直接退出。
- 打开浏览器用 `rundll32 url.dll,FileProtocolHandler <url>`，不引入 shell32 绑定。
- 字符串必须是 UTF-16（`windows.UTF16PtrFromString`）。

注意：`MessageBoxW` 在早于 COM/DPI 初始化时仍可用，这条路不会引入新的启动依赖。

### D5. 启动日志：追加一行 KV，超过 1 MB 截断

```
2026-10-18T09:12:03+08:00 stage=version-check os=6.3.9600 webview2=- error="unsupported Windows version"
```

- 路径 `%LOCALAPPDATA%\FlashDown\startup.log`，与配置目录同级但独立文件（不污染 `config.json`）
- 追加写；文件超过 1 MB 时先截断到 0（只保留最近一次启动的记录）
- 写日志失败只吞掉，不影响弹窗

### D6. 统一的失败出口

```go
// main.go
if err := startup.Check(...); err != nil {
    startup.Fail(err, info)      // 写日志 + 弹窗 + os.Exit(1)
}
...
if runErr != nil {
    startup.Fail(runErr, info)   // 替换 log.Fatal
}
```

`Fail` 内部：`log.Printf`（有控制台时可见）→ 写 `startup.log` → `MessageBoxW` → `os.Exit(1)`。

### D7. 检查顺序与成本

版本（微秒级）→ WebView2（一次注册表读 + 一次目录探测）→ `config.Load` → engine → Wails。
总开销在一次进程启动里可忽略，且都在 `application.New` 之前完成。

## Risks / Trade-offs

| 风险 | 影响 | 缓解 |
|---|---|---|
| 误判 WebView2 缺失，挡住能用的机器 | 高：软件直接不可用 | 三源检测 + 磁盘目录兜底；发布前在一台"卸载过 WebView2 又重装"的机器上验证 |
| `RtlGetVersion` 调用约定写错（stdcall/结构体大小） | 中：读到垃圾值 | 用 `x/sys/windows` 的 `NewLazySystemDLL` + 明确的结构体定义；单测覆盖解析逻辑，实机验证一次 |
| 弹窗在 CI/无桌面会话中阻塞 | 低：CI 不运行 GUI | 检查只在可执行体里，单测不调用 `Fail` |
| 日志暴露路径/版本等环境信息 | 低 | 仅记录版本与错误，不记录 URL、凭据、用户名 |
| `MB_OKCANCEL` 的"取消"语义不清 | 低 | 文案里写明"确定 = 打开下载页，取消 = 退出" |

## Migration Plan

无数据迁移。行为变化仅发生在失败路径：

- 之前：静默退出（退出码 1）
- 之后：弹窗 + 日志 + 退出码 1（脚本行为不变）

回滚方式：`main.go` 去掉两处 `startup.*` 调用即可，无持久化状态。

## Open Questions

- 是否需要 `--diagnose` 参数：不启动窗口，只做检查并把结果打到 stdout？（对远程支持有用，但可后置）
- WebView2 缺失时，除了打开下载页，是否要提供"我已安装，重新检测"按钮？`MessageBox` 不支持自定义按钮，需要 TaskDialog 或自绘窗口，成本上升。
