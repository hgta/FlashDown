## Context

FlashDown 是从零构建的桌面下载管理器，致敬 FlashGet。仓库当前只有 MIT LICENSE。

技术选型已锁定为 **Go 1.25+ / Wails v3（Beta）/ Vue 3 + TS + Vite**，范围限定 **M1–M4**（内核 + 多线程分块 + 任务管理 + 经典体验），协议限定 **HTTP/HTTPS**。

选型调研的关键发现：

- Wails **v2 是单窗口**，且无内置系统托盘 —— 悬浮窗与托盘是本项目刚需，v2 需多进程 + 第三方 systray 绕路
- Wails **v3 Beta（2026-08-02）原生支持多窗口与系统托盘**，且 Windows 端**纯 Go 不依赖 CGO**（本机无 gcc）
- v3 的 `WebviewWindowOptions` 提供 `Frameless` / `AlwaysOnTop` / `Hidden` / `Windows.HiddenOnTaskbar` —— 悬浮窗所需能力齐备
- 官方明确「v2 迁 v3 是一次真正的移植而非版本号升级」→ 从零开始直接上 v3，避免后期重写
- 硬门槛：**Wails v3 要求 Go 1.25+**，本机为 1.22.12 —— 已用 `GOTOOLCHAIN=auto` 自动获取工具链解决（实测 go1.26.9），**不改动本机 Go 安装**

### Spike 实测结果（2026-10-16，本机 Windows 11 23H2 / WebView2 155.0.4283.45）

| Spike | 结论 | 证据 |
|---|---|---|
| A · 多窗口事件广播 | ✅ **通过** | 12 秒内 broadcast 22 次结构化事件，`main` 与 `float` 两窗各回执 22 次，零丢失 |
| B · frameless 交互 | ⚠️ 机制已按真实契约接好，**待人工确认** | 无边框置顶窗口创建成功；`--wails-draggable` / `--custom-contextmenu` / `EnableFileDrop` 均按 v3 源码契约接入 |
| C · 并发 `WriteAt` | ✅ **通过** | 16384 块（64 MiB）打乱顺序写入：5 writer 与 30 writer 均 **0 块错位**，全文 SHA-256 与期望值一致 |
| C · 大文件预分配 | ⚠️ **推翻原假设** | 见 D4 |

`wails3 doctor` 全绿（`CGO_ENABLED=0`、WebView2 就位、npm 已装）；仅 NSIS 未安装，影响 6.5 打包。

## Goals / Non-Goals

**Goals:**

- 交付可实际使用的桌面下载器：多线程分块、真断点续传（进程崩溃/关机后能续）
- 一眼可辨的 FlashGet 布局，但用现代配色、高分屏适配与暗色主题重新渲染
- 下载引擎与 UI 彻底解耦，`internal/engine` 可无头运行、可单测
- 无广告、无捆绑、无云盘、无账号体系
- 单文件分发，冷启动快

**Non-Goals:**

- FTP / BT / 磁链 / MMS-RTSP（仅 HTTP/HTTPS）
- 浏览器扩展、镜像站点择优、计划任务、完成后关机、病毒扫描联动
- 移动端、Web 端、远程 RPC 控制
- 数据校验（MD5/SHA）
- 跨平台构建验证（代码层面保持可移植，但 MVP 只在 Windows 验证）

## Decisions

### D1. 技术栈 Go + Wails v3，`go.mod` 锁死版本

多窗口与托盘是刚需；v3 的 `app.Window.NewWithOptions()` 与 `app.SystemTray.New()` 直接满足，
Windows 端纯 Go 无需 gcc。代价是 Beta 稳定性风险 → 锁版本 + 开工前做两个 spike（见 Risks）。

### D2. 两层模型：Block（有状态）+ Jet（无状态），砍掉位图层

关键洞察：**HTTP Range 请求是字节级的，不是块级的。**
续传的唯一真相就是每个块的 `{Start, End, Done}` 三个整数，恢复时直接发
`Range: bytes=(Start+Done)-(End-1)`，粒度天然是字节。

因此不需要额外的 piece 位图层，模型塌缩为两层：

```
Block { Start, End, Done }   ← 持久化 / 调度 / UI 的唯一单位，落盘为 JSON 数组
Jet                          ← 纯执行、无状态、随时可死，从队列取块
```

Jet 无状态的收益：崩溃、暂停、动态调整线程数，都不需要恢复 Jet 状态，直接从块的 `Done` 重建队列。

### D3. 块划分取「固定块大小 + 工作队列」，不取动态劈分

- 块大小自适应：`blockSize = clamp(align64KB(total / (maxJets × 8)), 256KB, 8MB)`
- Jet 从共享队列领取**剩余字节最多**的块，跑完再领下一块（快的连接自然多干活）
- 块数约为 Jet 数的 8 倍，天然避免拖尾

放弃 FlashGet 原味的「动态劈分」作为主策略：劈分需要中断连接并重新发起 Range 请求，
增加复杂度与服务器压力，而固定块 + 队列在块数充足时效果等价。
**「智能分块」作为后续可选开关保留，MVP 不实现。**

### D4. 文件预分配：按**剩余空间**决策，而不是按文件系统类型

**原假设（已被 Spike C 推翻）**：NTFS 上 `Truncate` 不真占空间，只有 exFAT 才写零。

**实测结果**（目标卷 D:，NTFS）：

| 目标大小 | `Truncate` 耗时 | 实际占用磁盘 | 占比 | 末尾字节读出 |
|---|---|---|---|---|
| 256 MiB | 0 ms | 256 MiB | 100% | 0 ✓ |
| 1 GiB | 0 ms | 1 GiB | 100% | 0 ✓ |
| 4 GiB | 0 ms | 4 GiB | 100% | 0 ✓ |

即：**NTFS 上预分配的成本不是时间，而是空间。** 它亚毫秒完成（只预留簇、不写物理零，
未写区域读出为 0），但立即吃掉与文件等大的磁盘空间。

因此「自动」策略改为按剩余空间判定：
`预分配 ⟺ 目标卷剩余空间 ≥ 文件大小 × 1.2`。空间不足时不预分配（按需增长），
这是正常路径而非错误。另设一个绝对上限，避免在共享盘上瞬间吞掉几十 GB。

exFAT/FAT32 因本机只有 C:/D: 两个 NTFS 卷而**未能实测**；
保守起见 `auto` 只对 NTFS 启用，其余卷类型一律不预分配。

### D5. HTTP 客户端：强制 HTTP/1.1 + 关闭自动压缩

- `DisableCompression: true` —— Go 默认透明加 `Accept-Encoding: gzip` 并解压，会搅乱 Range 与 Content-Length 语义
- 先强制 HTTP/1.1（置空 `TLSNextProto`）—— HTTP/2 多路复用会让「N 连接 = N 份带宽」的模型失效，速率不可预测
- 停滞检测：包一层 reader，60 秒无数据即判失败并触发重试，避免永久挂住
- 共享 `http.Transport`，`MaxIdleConnsPerHost` 调大，`ResponseHeaderTimeout` 30s

### D6. 限速：TokenBucket，读之后扣减，三档取交集

全局桶 ∩ 任务桶 ∩ 时间段规则。在 `read` 之后 `Wait(n)` 扣减更准确；
burse 取 `rate × 100ms`，单次等待上限 100ms，避免低速时长时间 sleep 造成卡顿。

### D7. 事件节流：Engine 定时聚合，绝不逐次推送

15 个 Jet 每秒几十次回调，逐次推给前端会淹没 WebView2。

```
500ms → "tasks:snapshot"  全量任务快照，前端整体替换列表
250ms → "task:detail"     选中任务的块 / Jet 明细
1s    → "speed:tick"      当前速度 + 最近 60 点环形缓冲
```

主窗口与悬浮窗订阅同一批事件 —— 悬浮窗的速度数据零额外成本。

### D8. 前端 Vue 3 + TS + Vite

Wails 官方模板最成熟。任务网格用虚拟滚动（任务数可上千）；
块图用 `<canvas>` 而非上百个 DOM 节点（约 300 格，2–4 fps 重绘）。

### D9. 磁盘产物命名致敬 `.jc!`

```
<name>.fd!         数据文件
<name>.fd!.state   状态日志（JSON，原子写：写 .tmp → Sync → rename）
<name>             完成后重命名，并按类别移动到类别目录
```

状态落盘：每 1–3 秒或每完成一块时刷新，崩溃最多丢失一个落盘周期的进度。

### D10. 配置与历史放 `%APPDATA%\FlashDown`，不引数据库

`config.json`（选项 / 类别 / 下载规则）+ `history.jsonl`（追加写 + 启动时建内存索引）。
MVP 的数据量（数千条）用不上 sqlite，避免引入 cgo 依赖。

### D11. 悬浮窗交互全部走 Wails v3 的原生契约，不手搓 DOM

读 v3 源码（`internal/runtime/desktop/@wailsio/runtime/src/`）后确定三条契约：

| 能力 | 正确做法 | 常见误区 |
|---|---|---|
| 拖动无边框窗口 | CSS `--wails-draggable: drag`（值必须恰好是 `drag`） | v1/v2 的 `data-wails-drag` 在 v3 **无效** |
| 悬浮窗右键菜单 | CSS `--custom-contextmenu: <name>` + Go 侧 `application.NewContextMenu(name)` | 不必自绘 DOM 菜单——v3 直接弹**原生**菜单 |
| 屏蔽 WebView 默认菜单 | CSS `--default-contextmenu: hide` | **Debug 构建下无效**（v3 源码里 debug 直接 return），须在 release 构建验证 |

拖动只在按住**鼠标左键**并移动后才触发（`mousedown` 只是「武装」，`mousemove` 才真正开始），
所以单击不会误移动窗口；拖动过程中 `click` / `contextmenu` 会被抑制。

### D12. 拖放分两条独立通路，不能混为一谈

| 拖的来源 | 通路 | 需要的配置 |
|---|---|---|
| 资源管理器里的**文件** | Wails 原生 OS drop | 窗口 `EnableFileDrop: true` + 目标元素加 `data-file-drop-target` → 触发 `events.Common.WindowFilesDropped` |
| 浏览器地址栏/页面里的**链接** | 普通 HTML5 DOM drop，携带 `text/uri-list` | 无需 Wails 配置，但目标要 `preventDefault` 接管 `dragover`/`drop` |

「拖放篮」必须同时接这两条，否则从浏览器拖链接会静默失败——这是最容易被漏掉的一个坑。

## Risks / Trade-offs

| 风险 | 影响 | 缓解 |
|---|---|---|
| ~~Go 1.22.12 < 要求的 1.25+~~ | ~~阻塞开工~~ | ✅ 已解决：`GOTOOLCHAIN=auto` 自动获取 go1.26.9 |
| ~~spike A：多窗口事件广播~~ | ~~悬浮窗需另开 IPC 通道~~ | ✅ 已验证通过（22/22 双窗零丢失） |
| **spike B 的人工部分尚未确认** | 拖动 / 原生右键菜单 / 拖链接 / 拖文件四项需真人操作 | 机制已按 D11/D12 接入；失败则退回自绘菜单或独立进程悬浮窗 |
| ~~并发 `WriteAt` 正确性~~ | ~~多 Jet 写同一文件错位~~ | ✅ 已验证通过：5 与 30 writer 均 0 错位，SHA-256 一致 |
| **预分配会瞬间吃掉等大磁盘空间** | 20GB 任务在剩 15GB 的盘上会立刻失败 | 按剩余空间判定（D4）：不足则不预分配，按需增长 |
| **exFAT/FAT32 未实测** | 该卷类型上的预分配行为未知 | 本机只有 NTFS 卷；`auto` 策略只对 NTFS 启用 |
| **Wails v3 仍是 Beta** | API 变动、文档不全 | `go.mod` 与 `package.json` 双向锁死 `3.0.0-beta.30` |
| **块图渲染性能** | 300 格 DOM 高频重绘卡 UI | 用 `<canvas>`；必要时降到 2 fps |
| **`--default-contextmenu: hide` 在 Debug 构建下无效** | 开发期会看到 WebView 自带菜单 | 已知行为，release 构建验证，不为此加绕路代码 |
| **服务器不支持 Range** | 多线程失效 | 探测 `Accept-Ranges`；收到 `200` 而非 `206` 时整体降级为单连接从头下载，并在 UI 明确标记「不支持续传」 |
| **内容中途变更（ETag/Last-Modified 不一致）** | 续传后文件损坏 | 记录指纹；续传前校验，不一致则弹窗让用户选「重新下载 / 仍然继续」 |
| **杀毒软件抢占文件句柄** | 完成 rename 时失败 | rename 加重试与退避 |
| **配置/状态 JSON 损坏** | 任务丢失 | 原子写 + 损坏时备份原文件并降级为重新下载，不崩溃 |

## Migration Plan

全新项目，无迁移。实际初始化顺序（①–④ 已完成）：

1. 绕过 Go 版本门槛 → `GOTOOLCHAIN=auto` + `GOPROXY=https://goproxy.cn,direct`（本机 `proxy.golang.org` 不可达）
2. `go install .../wails3@latest` → v3.0.0-beta.30；`wails3 doctor` 全绿；`%USERPROFILE%\go\bin` 追加进用户 PATH
3. `wails3 init -t vue` 生成骨架（注意 `-d` 会再拼一层项目名，需手动搬平目录）
4. Spike A/B/C → 结论见 Context 表与 D4/D11/D12
5. 按 tasks.md 推进 M0–M4

## Open Questions

- 悬浮窗默认开启还是默认关闭？（倾向：首次启动时开，可在选项中关闭）
- 任务网格默认列集合与顺序是否完全照搬 FlashGet（文件名/大小/完成度/用时/剩余/速度/分块数）？
- 是否需要在 M4 提供「导入 FlashGet / aria2 未完成任务」的兼容层？（暂定不做）
