# Spec: startup-diagnostics

启动阶段的诊断能力：在创建任何窗口之前判断这台机器能不能跑，不能跑时把原因说清楚并留下记录。

## ADDED Requirements

### Requirement: 启动前校验 Windows 版本

应用 SHALL 在初始化 Wails 应用之前读取真实的 Windows 版本号（`RtlGetVersion`，而非注册表中的
`CurrentMajorVersionNumber`），并与最低支持版本 `10.0.17763` 比较。

版本低于下限时，应用 MUST 阻止启动、显示可见错误、以非零码退出，且 MUST NOT 尝试创建窗口。

#### Scenario: Windows 8.1 上启动
- **WHEN** 用户在 Windows 8.1（6.3.9600）上双击 `flashdown.exe`
- **THEN** 不显示主窗口，弹出错误对话框，正文包含当前系统版本 `6.3.9600` 与要求 `Windows 10 1809 或更高版本`

#### Scenario: Windows 10 满足下限
- **WHEN** 用户在 Windows 10 build 19045 上启动
- **THEN** 版本校验通过，继续执行 WebView2 检测

#### Scenario: 版本读取失败时放行
- **WHEN** `RtlGetVersion` 调用失败或返回全零
- **THEN** 视为未知版本并**放行**，同时在启动日志中记录读取失败，不得因读取失败而阻止启动

### Requirement: 启动前校验 WebView2 运行时

应用 SHALL 在创建窗口之前确认 WebView2 运行时可用。检测顺序为：
1. 注册表 `HKLM\SOFTWARE\Microsoft\EdgeUpdate\Clients\{F3017226-FE2A-4295-8BDF-00C3A9A7E4C5}` 的 `pv`
2. 注册表 `HKLM\SOFTWARE\WOW6432Node\Microsoft\EdgeUpdate\Clients\{F3017226-FE2A-4295-8BDF-00C3A9A7E4C5}` 的 `pv`
3. 磁盘目录 `%ProgramFiles(x86)%\Microsoft\EdgeWebView\Application\<版本>`

任一来源给出有效的非空版本号即视为可用。全部来源都无结果时，MUST 阻止启动并提示安装 WebView2。

#### Scenario: 缺少 WebView2
- **WHEN** 注册表两处均无有效 `pv`，且磁盘上不存在 `Microsoft\EdgeWebView\Application` 版本目录
- **THEN** 弹出错误对话框，说明缺少 Microsoft Edge WebView2 运行时，并提供微软官方下载地址

#### Scenario: 只有磁盘目录存在
- **WHEN** 注册表读取不到版本，但磁盘上存在 `Microsoft\EdgeWebView\Application\109.0.1518.78`
- **THEN** 视为可用并继续启动，不得误判为缺失

### Requirement: 启动失败必须可见

任何启动期失败（版本过低、缺少 WebView2、`wailsApp.Run()` 返回错误）MUST 在退出前向用户显示模态消息框，
内容 MUST 包含三部分：失败原因、当前系统版本、启动日志路径。

进程 MUST 以非零码退出。若 stderr 可写（从控制台启动），MUST 同时输出同样的信息。

#### Scenario: WebView2 缺失时的提示内容
- **WHEN** 前置检查判定 WebView2 缺失
- **THEN** 对话框标题为 `FlashDown 无法启动`，正文含缺失原因与 `startup.log` 的完整路径

#### Scenario: Run 失败不再是静默
- **WHEN** `wailsApp.Run()` 返回非 nil 错误
- **THEN** 错误写入启动日志并弹出对话框显示该错误的文本，进程退出码非零

#### Scenario: 主窗口不出现
- **WHEN** 任一项前置检查失败
- **THEN** 整个过程不创建主窗口，也不出现"窗口一闪而过"

### Requirement: 启动失败留痕

每个失败路径 SHALL 向 `%LOCALAPPDATA%\FlashDown\startup.log` 追加一条记录，包含时间戳、系统版本
（major.minor.build）、WebView2 检测值（版本号或"未找到"）、失败阶段、原始错误文本。

日志写入失败时 MUST NOT 影响消息框的展示。

#### Scenario: 8.1 上留下日志
- **WHEN** 应用因版本过低退出
- **THEN** `startup.log` 中新增一条记录，含 `6.3.9600` 与阶段 `version-check`

#### Scenario: 日志不可写
- **WHEN** `%LOCALAPPDATA%` 不可写或磁盘已满
- **THEN** 仍然弹出错误对话框，进程正常以非零码退出，不因写日志失败而崩溃

### Requirement: 声明系统要求

项目的 README、GitHub release 说明、以及发布 zip 内的 `SYSTEM-REQUIREMENTS.txt` MUST 声明：
Windows 10 1809（build 17763）或更高 / Windows 11、64 位、需要 WebView2 运行时，并明确 Windows 7 / 8 / 8.1 不受支持。
文件名使用 ASCII，内容可用中文，以免旧解压工具把文件名显示成乱码。

#### Scenario: 下载后即可看到要求
- **WHEN** 用户解压发布包
- **THEN** 压缩包内包含 `SYSTEM-REQUIREMENTS.txt`，内容与 README 的"系统要求"一节一致

#### Scenario: 发布页说明
- **WHEN** 用户打开 v0.2.0 及之后版本的 release 页面
- **THEN** 说明中列出系统要求与"Windows 8.1 及更早版本不受支持"的明确说明
