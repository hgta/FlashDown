## 1. 诊断包 `internal/startup`

- [x] 1.1 封装 `RtlGetVersion`：`NewLazySystemDLL("ntdll.dll")` + `RTL_OSVERSIONINFOW` 结构体，返回 `major.minor.build`
- [x] 1.2 版本解析单测：正常值、全零、调用失败三种输入都能得到预期结果
- [x] 1.3 WebView2 三源检测：两处注册表 `pv` + `%ProgramFiles(x86)%\Microsoft\EdgeWebView\Application` 目录探测，读取函数以参数注入
- [x] 1.4 WebView2 检测单测：注册表命中、仅 WOW6432Node 命中、仅磁盘命中、全部落空四种情形
- [x] 1.5 `Check` 组合判断：低于 10.0.17763 拦截、版本未知放行、WebView2 缺失拦截；表驱动覆盖
- [x] 1.6 `Fail`：`log.Printf` + 追加 `startup.log` + `MessageBoxW` + `os.Exit(1)`；写日志失败不影响弹窗
- [x] 1.7 日志格式单测：注入 `io.Writer`，校验 KV 字段与时间戳，超过 1 MB 时截断

## 2. 接入启动流程

- [x] 2.1 `main.go` 在 `config.Load()` 之前调用 `startup.Check`，失败调 `startup.Fail`
- [x] 2.2 `wailsApp.Run()` 的错误由 `log.Fatal` 改为 `startup.Fail`，保持非零退出码
- [x] 2.3 stderr 可写时同时输出同样的信息，避免控制台启动时"只有弹窗没有文本"

## 3. 发布侧声明

- [x] 3.1 `README.md` 增加"系统要求"一节：Windows 10 1809+ / Windows 11、x64、需要 WebView2；明确 Win7/8/8.1 不支持
- [x] 3.2 `RELEASE_NOTES.md` 增加同样的要求与"8.1 上会发生什么"的说明
- [x] 3.3 CI 打包步骤把 `SYSTEM-REQUIREMENTS.txt` 一并放进 zip

## 4. 验证

- [x] 4.1 `go vet ./...`、`go test ./internal/startup ./internal/engine` 全绿
- [ ] 4.2 Windows 8.1 x64 实机（或虚拟机）：双击后出现弹窗，正文含 `6.3.9600` 与日志路径；`startup.log` 有对应记录
- [x] 4.3 Windows 10/11 正常启动：不出现任何提示，主窗口照常
- [ ] 4.4 在 Win10 上模拟 WebView2 缺失（临时改注册表或改名目录）：出现安装引导，点击确定能打开下载页
