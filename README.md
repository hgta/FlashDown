# FlashDown

致敬 FlashGet 的桌面下载管理器：Go + Wails v3 + Vue 3，单文件可执行，无广告、无捆绑。

多连接分块下载、断点续传、进程重启恢复、块图与 Jet 面板——FlashGet 那套"看得见分块在跑"的形态，用现代技术栈重做一遍。

## 系统要求

| 项目 | 要求 |
|---|---|
| 系统 | **Windows 10 1809（build 17763）或更高**，或 Windows 11 |
| 架构 | 64 位（x64） |
| 运行时 | Microsoft Edge WebView2（Windows 10 / 11 通常已内置） |
| 磁盘 | 约 20 MB（不含下载内容） |

**不支持 Windows 7 / 8 / 8.1。** Go 1.21+ 与 Wails v3 均以 Windows 10 为下限，WebView2 对 8.1 的支持也已于 2024 年结束。在不支持的系统上启动时，程序会弹窗说明原因，并把详情写入：

```
%LOCALAPPDATA%\FlashDown\startup.log
```

## 下载

从 [Releases](https://github.com/hgta/FlashDown/releases) 取得 `flashdown-<version>-win64.zip`，解压后双击 `flashdown.exe`。

## 功能

- 多连接分块下载，默认 5 条连接，可在「任务 → 连接数」切换 1 / 3 / 5 / 8 / 16 / 30，**下载途中调整立即生效**
- 断点续传：暂停、退出、重启后从断点继续，字节级校验
- 重定向下载：跟随 302 直连资源地址；签名地址过期自动重新获取
- 服务器不支持 Range 时降级单连接，并在任务上标记「不可续传」
- 失败自动重试（默认 5 次，指数退避），60 秒无数据判定卡死
- 六区经典布局：菜单栏、工具栏、类别树、任务网格、详情面板（块图 / Jet / 日志）、状态栏
- 拖动链接到窗口即可新建任务；`Ctrl+N` 打开新建对话框
- 明暗主题跟随系统

## 开发

需要 Go 1.25、Node 20+、[wails3 CLI](https://v3.wails.io/)（版本须与 `go.mod` 一致）。

```powershell
wails3 dev                     # 热重载开发模式
wails3 build                   # 生产构建，产物在 bin/
go test ./...                  # 后端测试
cd frontend; npm run build     # 前端类型检查 + 打包
```

前端绑定（`frontend/bindings/`）由 `wails3 generate bindings -ts -i` 生成并提交；改了 Go 侧服务方法后需要重新生成。

## 打包发布

推送 `v*` tag 会触发 `.github/workflows/release.yml`：在 `windows-latest` 上构建、压缩、创建 GitHub Release。

```powershell
git tag -a v0.2.0 -m "FlashDown v0.2.0"
git push origin v0.2.0
```

## 结构

```
main.go                  启动、窗口装配、平台检查
app/                     Wails 服务层（任务、配置、事件广播）
internal/engine/         下载内核：分块、Jet、续传、重试（与 UI 无关）
internal/config/         配置加载与持久化
internal/startup/        启动诊断：系统版本、WebView2、失败弹窗与日志
frontend/                Vue 3 界面
openspec/                变更提案与规格
```

## 已知限制

- 仅 Windows x64
- 批量 URL 预览、限速、类别规则编辑、计划任务尚未实现
- 多连接对单 IP 限速的服务器无提速效果
