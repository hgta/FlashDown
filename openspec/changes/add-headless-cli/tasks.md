## 1. 共享与平台判断

- [x] 1.1 新增 `internal/version`，`app.Version` 改为引用它
- [x] 1.2 `internal/startup` 增加 `Mode`（GUI / CLI）与 `CheckFor`，`Check` 保持为 GUI 别名
- [x] 1.3 CLI 模式：跳过 WebView2、版本过低只警告并写 `stage=platform-warning`
- [x] 1.4 `Info.Unsupported` 字段与单测（8.1 + CLI 放行、8.1 + GUI 拦截）

## 2. 命令行程序

- [x] 2.1 `internal/cli`：参数解析（url / -o / -n / -j / -q），用法错误返回 2
- [x] 2.2 编排：config → engine → Add → Start → 轮询状态 → 退出码
- [x] 2.3 进度渲染：TTY 单行刷新、非 TTY 每 10% 一行、`-q` 静默
- [x] 2.4 Ctrl+C：Pause → 等 paused → Shutdown → 退出 130
- [x] 2.5 `cmd/flashdown-cli/main.go`：只依赖 engine/config/startup/version，不 import Wails
- [x] 2.6 单测：参数解析、进度格式、退出码映射

## 3. GUI 引导

- [x] 3.1 旧系统拒绝文案加入 `flashdown-cli.exe` 与示例命令
- [x] 3.2 文案单测（包含 `flashdown-cli.exe`）

## 4. 发布与文档

- [x] 4.1 CI 增加 CLI 构建步骤，zip 内放入 `flashdown-cli.exe`
- [x] 4.2 `README.md` 增加"命令行模式"一节与两种入口的系统要求
- [x] 4.3 `SYSTEM-REQUIREMENTS.txt` 说明 GUI 与 CLI 的差异
- [x] 4.4 `RELEASE_NOTES.md` 增加本版说明

## 5. 验证

- [x] 5.1 `go vet`、`go test ./...` 全绿
- [x] 5.2 本地 `flashdown-cli.exe` 下载一个真实 URL，检查进度、退出码 0
- [x] 5.3 Ctrl+C 中断后重跑，确认续传且字节一致
- [ ] 5.4 Windows 8.1 实机：CLI 下载成功；GUI 弹窗提示使用 CLI（无实机，未验证）

## 实现备注

- 5.3 的验证方式：在支持 Range 的源上预置 `<文件名>.fd!` 与 `.state`（5 MB / 26.9 MB），
  期望首行打印「继续未完成的下载」，进度起点 ≈17.8%，完成后 SHA256 与 `curl` 直取一致。
- 调查中发现并修复：CLI 每次启动都 `Add` 新任务，从不读取磁盘日志，导致"重跑同一条命令即续传"
  实际从 0 开始。现由 `Manager.RestoreDir` + `Manager.Find` 在 `Add` 之前恢复。
