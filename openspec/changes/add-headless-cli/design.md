## Context

8.1 实机反馈（v0.2.1）：

```
弹窗正常
startup.log: 2026-10-18T08:58:25+08:00 stage=version-check os=6.3.9600 webview2=- error="..."
```

这两条把不确定性清掉了：Go 1.25 编出的二进制在 8.1 上能跑，`RtlGetVersion` 也对，失败的只有 WebView2 那一层。

同时，`internal/engine` 的依赖面很小：

```
engine ──▶ config (stdlib)
       ──▶ golang.org/x/sys/windows (GetDiskFreeSpaceEx)
```

没有一行 Wails。所以"能不能下载"和"能不能开窗口"是两个独立问题，之前被绑在一起了。

## Goals / Non-Goals

**Goals:**

- 同一份下载内核，提供一个不依赖 WebView2 的入口，8.1 上可用
- 命令行的行为可预期：进度、退出码、中断、续传
- GUI 在旧系统上的拒绝不再是一条死路，而是指向 CLI
- 发布包保持"下载即用"，两个 exe 在同一 zip 内

**Non-Goals:**

- 不支持 Windows 7（没有验证环境，也不打算承诺）
- 不做图形界面降级（Win32 重写另一个量级）
- 不做守护进程 / 本地 HTTP 服务（浏览器界面是后续可选项）
- 不改引擎的下载语义（CLI 只是另一个调用方）

## Decisions

### D1. 独立 main 包，绝不 import Wails

`cmd/flashdown-cli` 只依赖 `internal/engine`、`internal/config`、`internal/startup`、`internal/version`。

原因不是洁癖：Go 的包初始化在 `main()` 之前执行，若同一个 exe 里 import 了 Wails，`application.RegisterEvent` 之类的 init 路径就无法跳过。8.1 上的不确定性必须压到零，所以两个入口 = 两个二进制。

副作用是正面的：CLI 体积约 8 MB（GUI 是 12 MB，含前端资源）。

### D2. startup 分模式：GUI 严格，CLI 宽松

```go
type Mode int
const (ModeGUI Mode = iota; ModeCLI)

CheckFor(mode, readVersion, readWebView) (Info, error)
```

| 检查 | ModeGUI | ModeCLI |
|---|---|---|
| 版本 < 10.0.17763 | 拦截 | 放行，`Info.Unsupported=true`，记日志 |
| WebView2 缺失 | 拦截 | 不检查 |
| 版本读取失败 | 放行 | 放行 |

`Check(...)` 保留为 `CheckFor(ModeGUI, ...)` 的别名，避免改动现有调用与测试。

CLI 的日志仍写 `startup.log`，但用 `stage=platform-warning`，且**不弹窗**——命令行用户需要的是继续执行并给出一行提醒。

### D3. CLI 复用引擎的调用方式

```go
eng := engine.New(cfg)      // cfg 来自 config.Load()，8.1 首次运行会生成默认值
task, _ := eng.Add(engine.Spec{URL, Dir, Name, Category})
eng.Start(task.ID)
```

轮询 `task.View()` 与 `task.State()`（200 ms）即可拿到进度与终态，不需要事件总线。

续传依赖现有语义：`Name` 留空时探测后按 URL/Content-Disposition 取名，与上次相同 → `Paths()` 命中同一个 `.fd!` + `.state` → `reconcile` 续传。用户用 `-n` 指定名字时同理。

### D4. 进度输出：TTY 单行，非 TTY 行式

```
 vtplay-0.3.0-win64.zip  103.5 MB  5 连接
  47.3%  2.8 MB/s  剩余 00:19
```

- stdout 是字符设备（TTY）→ 用 `\r` 原地刷新，每秒最多 4 次
- 重定向到文件/管道 → 每 10% 打一行，避免日志被刷爆
- `-q` → 只输出最终路径与错误到 stderr

TTY 判定用 `os.Stdout.Stat()` 的 `ModeCharDevice`，不引入 `x/term`。

### D5. 中断：先落盘，再退出

`signal.NotifyContext(ctx, os.Interrupt)`：

```
Ctrl+C → ctx 取消 → eng.Pause(id) → 等 state 变成 paused（最多 5 s）→ eng.Shutdown(2s) → 退出 130
```

退出码 130（128+SIGINT）是 shell 惯例，也方便脚本判断"这次是被人打断的，可以续"。

### D6. 退出码约定

| 码 | 含义 |
|---|---|
| 0 | 下载完成 |
| 1 | 下载失败（网络、磁盘、服务器错误） |
| 2 | 用法错误（缺少 URL、参数非法） |
| 130 | 用户中断，进度已保存 |

### D7. 版本常量下沉

`app.Version` 被 CLI 需要，但 `app` 包 import 了 Wails。把常量移到 `internal/version`：

```go
package version
const Value = "0.3.0"
```

`app` 与 `cli` 各自引用，`const Version = version.Value` 保持 API 不变。

## Risks / Trade-offs

| 风险 | 影响 | 缓解 |
|---|---|---|
| 8.1 不属于 Go 官方支持范围 | 未来工具链可能失效 | 文案写"尽力支持"；CI 无法覆盖 8.1，只能靠用户实机回归 |
| GUI 与 CLI 同时下载同一文件 | journal 互相覆盖 | CLI 检测 `.state` 存在时提示"另一实例可能在用"，需要 `-f` 才继续（后续可加） |
| 中文路径/文件名在 8.1 控制台 | 输出乱码 | 输出前把非 ASCII 转义？代价高；先接受，README 提示 `chcp 65001` |
| CLI 与 GUI 的配置共享 | CLI 改 `-j` 会写回 config | `-j` 只作用于本次进程，不持久化 |

## Migration Plan

无迁移。GUI 行为不变，只是多了一个同级 exe 与一条文案。

回滚：删除 `cmd/flashdown-cli` 与 CI 的构建步骤即可。

## Open Questions

- 是否需要 `--resume-only`（只续传不新建）？
- 是否需要把进度写到 `--log` 文件，供计划任务使用？
