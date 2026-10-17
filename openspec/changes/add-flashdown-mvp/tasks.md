## 0. 前置与 Spike（阻塞后续全部工作）

- [x] 0.1 Go 1.25+ 就绪 —— 未改动 `D:\vip\tools\go`（1.22.12），改用 `GOTOOLCHAIN=auto` 按需自动获取工具链；并把 `GOPROXY` 持久化为 `https://goproxy.cn,direct`（`proxy.golang.org` 在本机不可达）。实测生效工具链为 go1.26.9
- [x] 0.2 安装 Wails CLI `wails3 v3.0.0-beta.30`，`wails3 doctor` 全绿（`SUCCESS No issues found`）；WebView2 155.0.4283.45 已就位、CGO_ENABLED=0；并将 `%USERPROFILE%\go\bin` 追加到用户 PATH
- [x] 0.3 Spike A 双窗口事件广播 —— **通过**：12 秒 emit 22 次，`main` 与 `float` 各回执 22 次，零丢失；结构化载荷 `RegisterEvent[SpeedTick]` 可用
- [x] 0.4 Spike B frameless 交互 —— **已获证据**：无边框置顶窗创建成功；`--wails-draggable: drag` 可拖动；`--custom-contextmenu` + 原生 `NewContextMenu` 弹出且点击后回调进 Go（记录到「全部开始」）；HTML5 drop 成功捕获浏览器拖入的链接（记录到完整 URL）。`fileDropSeen` 仍为 false —— 原生**文件**拖放未验证，但 MVP 主流程只需链接拖放，不作为门槛
- [x] 0.5 Spike C 并发写盘与预分配 —— **并发通过**（5 与 30 writer 均 0 块错位、SHA-256 一致）；**预分配推翻原假设**：NTFS 上 `Truncate` 亚毫秒完成但立即占用等大空间（256MiB/1GiB/4GiB 全部 100% 占用），故策略改为按剩余空间判定；exFAT 因本机无该卷类型未实测
- [x] 0.6 依据 spike 结论回写 `design.md` —— 已完成：新增 D11（悬浮窗原生契约）、D12（拖放双通路），改写 D4（预分配按空间决策），更新 Context 与风险表

## 1. M0 工程骨架

- [x] 1.1 `wails3 init -t vue` 生成骨架；`go.mod` = `go 1.25.0` + 锁死 `wails/v3 v3.0.0-beta.30`；前端 `@wailsio/runtime` 已从 `latest` 改为固定 `3.0.0-beta.30`；已跑通 `generate bindings` → `vue-tsc` → `vite build` → `go build` 全链路
- [ ] 1.2 建立目录骨架：`app/`、`internal/engine`、`internal/config`、`internal/history`、`internal/batch`、`internal/monitor`、`internal/platform`
- [x] 1.3 主窗口静态骨架：菜单栏 + 工具栏 + 类别树 + 任务网格 + 底部面板 + 状态栏（六区布局）—— 已完成：`src/windows/MainFrame.vue` 六区 + 两条可拖动分隔条（比例持久化走 `SaveLayout`），`components/` 下 MenuBar / ToolBar / CategoryTree / TaskGrid / DetailPane / StatusBar / Sparkline；任务网格虚拟滚动、表头排序、列显隐；底部面板三标签（块图 `<canvas>` / Jet / 日志）；`vue-tsc` + `vite build` 通过
- [ ] 1.4 主题 `tokens.css`：明暗变量、高 DPI 适配、复刻 FlashGet 的信息密度与间距
- [ ] 1.5 配置模块：`%APPDATA%\FlashDown\config.json` 读写、默认值与损坏回退
- [ ] 1.6 首次启动初始化默认类别：软件 / 游戏 / 驱动程序 / MP3 及各目录
- [ ] 1.7 事件骨架：`tasks:snapshot`(500ms) / `task:detail`(250ms) / `speed:tick`(1s) 与节流 ticker

## 2. M1 单连接可续传内核

- [x] 2.1 定义 `Block{Start,End,Done}` 与 Journal 数据结构 —— `internal/engine/block.go`、`journal.go`
- [x] 2.2 HTTP 探测：获取 totalSize、`Accept-Ranges`、`ETag`、`Last-Modified` —— `probe.go`；用 `Range: bytes=0-0` 而非 HEAD（HEAD 的 Accept-Ranges 不可信），200/206/416 分别处理
- [x] 2.3 FileWriter：预分配策略（auto/always/never）+ `WriteAt` + offsetWriter 封装 —— `writer.go`；`auto` 按 D4 实现：仅 NTFS 且剩余空间 ≥ 1.2×文件大小才 Truncate
- [x] 2.4 单连接下载循环：256KB 缓冲池 + `DisableCompression` + 强制 HTTP/1.1 —— `client.go` + `runner.go`
- [x] 2.5 Journal 原子写：`.fd!.state`（tmp → Sync → rename），1–3s 与块完成时刷新，损坏则备份并降级 —— 实为 1s；损坏备份为 `.bak` 并返回 `ErrJournalCorrupt`
- [x] 2.6 任务状态机：Draft/Waiting/Probing/Downloading/Paused/Verifying/Completing/Finished/Error —— `task.go`，含 Waiting（并发槽位排队）
- [x] 2.7 暂停/继续：中断连接 → 落盘 `Done` → 按字节偏移续传 —— 另含进程重启恢复：`Manager.Restore()` 扫描 `.fd!.state` 重建为暂停态
- [x] 2.8 Range 降级：收到 200 而非 206 时整体转单连接从头下载并标记「不支持续传」 —— 仅当请求起点 > 0 才降级，起点为 0 的 200 直接消费，避免降级死循环
- [ ] 2.9 续传前校验 ETag/Last-Modified，不一致时弹窗让用户选重新下载或继续 —— 校验与拒绝已完成（`ErrRemoteChanged`，`Manager.Restart` 即「重新下载」）；缺的是弹窗与「仍然继续」分支
- [x] 2.10 停滞检测：60 秒无数据判为失败 —— `stallGuard`，先置标志再 cancel，否则与用户暂停无法区分
- [x] 2.11 错误分类（可重试 / 终止）与可配置重试次数、退避间隔 —— `errors.go`；退避 = 间隔×第几次，上限 30s
- [x] 2.12 完成处理：大小校验 → rename → 移入类别目录 → 冲突加数字后缀 → 清理 `.fd!`；rename 被占用时退避重试 —— 文件直接写入类别目录，故无「移入」步骤；rename 重试 5 次
- [ ] 2.13 新建任务对话框：URL / 另存名 / 目录 / 类别 / 块数 / 引用页 / 登录 / 注释 / 开始方式
- [ ] 2.14 暴露 Go API 绑定并在前端接入添加、暂停、继续、删除 —— `main.go` 已把引擎注入 `app.Provider` 与 `Service.OnConfigSaved`，只差绑定层的 Add/Pause/Resume/Remove
- [x] 2.15 引擎单元测试：本地 HTTP server 覆盖续传、降级、重试（无窗口运行） —— 17 个用例，`go test ./internal/engine/ -count=2` 通过

## 3. M2 多线程分块

- [ ] 3.1 自适应块大小：`clamp(align64KB(total/(maxJets×8)), 256KB, 8MB)`
- [ ] 3.2 Planner：共享队列，jet 领取「剩余字节最多」的块
- [ ] 3.3 Jet 并发循环：领块 → Range 请求 → 边读边 `WriteAt` → 还块 → 领下一块
- [ ] 3.4 运行中动态调整 jet 数（默认 5，上限 30）
- [ ] 3.5 块图 `<canvas>` 组件：约 300 格，按 jet 着色，2–4 fps 重绘
- [ ] 3.6 Jet 列表组件：区间、进度条、百分比、速度
- [ ] 3.7 全局并发上限（默认 3）与等待队列按序启动
- [ ] 3.8 大流量下多 jet 并发写盘正确性验证

## 4. M3 任务管理、限速、类别与批量

- [ ] 4.1 TokenBucket：全局 ∩ 任务，读后扣减，burst = rate×100ms，单次等待 ≤100ms
- [ ] 4.2 时间段限速规则，跨规则边界重算且不中断连接
- [ ] 4.3 滑动窗口测速（约 5s）与剩余时间估算（速度为 0 显示「未知」）
- [ ] 4.4 速度曲线环形缓冲（60 点）
- [ ] 4.5 类别管理 UI：增删改名、目录选择；删除类别时任务改归属而非删文件
- [ ] 4.6 下载规则（扩展名 → 类别）与规则管理 UI（排序、启停）
- [ ] 4.7 通配符解析：`(*)` + 通配符长度、数字/字母区间、步长 `(1-20:2)`
- [ ] 4.8 多括号笛卡尔积展开
- [ ] 4.9 批量任务预览对话框：全量列表、计数、可取消勾选，确认后才入队
- [ ] 4.10 任务排序、上移下移、全部开始 / 全部暂停
- [ ] 4.11 删除进「已删除」节点与还原；区分仅删任务与删除文件

## 5. M4 经典体验

- [ ] 5.1 悬浮窗：`Frameless` + `AlwaysOnTop` + `HiddenOnTaskbar`，约 220×90
- [ ] 5.2 悬浮窗拖动与位置持久化（重启后复原，换显示器时钳回屏幕内）
- [ ] 5.3 悬浮窗速度数字 + 60 点滚动曲线
- [ ] 5.4 拖放篮：接收拖拽 URL → 预填新建任务对话框；非 URL 给内联提示
- [ ] 5.5 悬浮窗自绘右键菜单：显示/隐藏主窗口、新建、全部开始、全部暂停、设置、退出
- [ ] 5.6 系统托盘：明暗图标、右键菜单
- [ ] 5.7 关闭主窗口隐藏到托盘并保持下载；退出需显式操作且先落盘
- [ ] 5.8 完成通知：窗口隐藏或失焦时触发，可在设置中关闭
- [ ] 5.9 剪贴板监视：URL 识别、与上次相同内容抑制、已下载 URL 抑制、自动添加模式
- [ ] 5.10 日志面板：记录状态流转、错误、重试与 HTTP 响应
- [ ] 5.11 历史记录 `history.jsonl` 追加写 + 启动建内存索引
- [ ] 5.12 查找：按文件名/URL 跨活动任务与历史
- [ ] 5.13 新建时命中历史则提示「已下载过」并显示原路径与日期
- [ ] 5.14 导入/导出未完成任务；导出选中 URL 列表（纯文本，一行一条）

## 6. 收尾与验证

- [ ] 6.1 端到端验证：1GB 文件 5 jet 下载 → 暂停 → 杀进程 → 恢复 → 完成校验字节一致
- [ ] 6.2 1000 任务网格虚拟化性能验证（滚动流畅、不掉帧）
- [ ] 6.3 高 DPI 125% / 150% / 200% 视觉检查
- [ ] 6.4 `openspec validate add-flashdown-mvp --strict` 通过
- [ ] 6.5 打包 Windows 安装包并冒烟测试（安装 → 首次启动 → 下一个文件 → 卸载）；前置：安装 NSIS（`wails3 doctor` 显示当前未安装）
