# FlashDown

致敬 FlashGet 的桌面下载管理器：Go + Wails v3 + Vue 3，单文件可执行，无广告、无捆绑。

多连接分块下载、断点续传、进程重启恢复、块图与 Jet 面板——FlashGet 那套"看得见分块在跑"的形态，用现代技术栈重做一遍。

## 系统要求

| 入口 | 系统 | 运行时 |
|---|---|---|
| `flashdown.exe`（图形界面） | **Windows 10 1809（build 17763）或更高**，或 Windows 11 | Microsoft Edge WebView2（Windows 10 / 11 通常已内置） |
| `flashdown-cli.exe`（命令行） | **Windows 7 SP1 及以上** | 无 |

两者均为 64 位（x64），磁盘占用约 20 MB（不含下载内容）。

图形界面**不支持 Windows 7 / 8 / 8.1**：Go 1.21+ 与 Wails v3 均以 Windows 10 为下限，WebView2 对 8.1 的支持也已于 2024 年结束。在这些系统上启动时会弹窗说明原因（并建议改用 `flashdown-cli.exe`），详情写入：

```
%LOCALAPPDATA%\FlashDown\startup.log
```

## 下载

从 [Releases](https://github.com/hgta/FlashDown/releases) 取得 `flashdown-<version>-win64.zip`，解压得到两个入口：

- 双击 `flashdown.exe` 使用图形界面
- 在命令行运行 `flashdown-cli.exe` 直接下载

## 命令行模式

没有图形环境、或者系统太旧跑不了 WebView2 时，用 `flashdown-cli.exe`。它复用同一个下载内核（分块、多连接、断点续传、重定向、重试），只是不画界面。

```powershell
flashdown-cli.exe <链接> [-o 目录] [-n 文件名] [-j 连接数] [-c 类别] [-q]

flashdown-cli.exe https://example.com/big.iso -o D:\DL -j 8
```

| 参数 | 说明 |
|---|---|
| `-o <目录>` | 保存目录，默认取配置里的下载目录 |
| `-n <文件名>` | 指定文件名，默认由 URL 或响应头推断 |
| `-j <数量>` | 连接数 1–30，默认取配置值；仅对大于 256 KB 的文件有意义 |
| `-c <类别>` | 类别 id |
| `-q` | 不输出进度 |
| `--version` | 显示版本 |

行为约定：

- 标志与链接可以任意顺序混排（`-j 8 URL`、`URL -j 8` 均可），`--` 之后的全部当作链接
- `Ctrl+C` 中断后进度落盘，**重跑同一条命令即从断点继续**
- 退出码：`0` 完成 / `1` 下载失败 / `2` 用法错误 / `130` 已中断（可续传）

前台运行时进度是单行刷新；输出被重定向到文件或管道时，每 10% 打一行，避免刷屏。

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

推送到 `v*` tag 会触发 `.github/workflows/release.yml`：在 `windows-latest` 上跑测试、构建 GUI 与 CLI 两个可执行文件、压缩、创建 GitHub Release。

```powershell
git tag -a v0.3.0 -m "FlashDown v0.3.0"
git push origin v0.3.0
```

## 结构

```
main.go                  启动、窗口装配、平台检查
cmd/flashdown-cli/       无界面入口（不 import Wails）
app/                     Wails 服务层（任务、配置、事件广播）
internal/cli/            命令行参数解析与进度渲染
internal/engine/         下载内核：分块、Jet、续传、重试（与 UI 无关）
internal/config/         配置加载与持久化
internal/startup/        启动诊断：系统版本、WebView2、失败弹窗与日志
internal/version/        版本号（GUI 与 CLI 共用的唯一来源）
frontend/                Vue 3 界面
openspec/                变更提案与规格
```

## 已知限制

- 仅 Windows x64
- 批量 URL 预览、限速、类别规则编辑、计划任务尚未实现
- 多连接对单 IP 限速的服务器无提速效果
