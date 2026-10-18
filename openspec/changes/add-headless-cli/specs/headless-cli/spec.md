# Spec: headless-cli

无界面下载入口：在没有 WebView2、甚至不受官方支持的系统上，仍然把文件下下来。

## ADDED Requirements

### Requirement: 命令行入口

`flashdown-cli.exe` SHALL 接受一个位置参数作为下载链接，并支持 `-o` 保存目录、`-n` 文件名、
`-j` 连接数、`-q` 静默四个选项。

缺少链接或链接不是 http/https 时 MUST 输出用法并以退出码 2 结束。

#### Scenario: 最简调用
- **WHEN** 用户执行 `flashdown-cli.exe https://example.com/a.zip`
- **THEN** 文件保存到配置中的默认下载目录，文件名由服务器或 URL 推断

#### Scenario: 指定目录与文件名
- **WHEN** 用户执行 `flashdown-cli.exe https://example.com/a.zip -o D:\DL -n b.zip`
- **THEN** 结果落在 `D:\DL\b.zip`

#### Scenario: 缺少链接
- **WHEN** 用户不带任何参数执行
- **THEN** 打印用法，退出码为 2，不发起任何网络请求

### Requirement: 平台降级

CLI SHALL 在 Windows 版本低于 10.0.17763 时继续运行，MUST NOT 检查 WebView2 运行时。

降级运行时 MUST 在 `%LOCALAPPDATA%\FlashDown\startup.log` 记录一条 `stage=platform-warning`，
且 MUST NOT 弹出模态对话框。

#### Scenario: Windows 8.1 上下载
- **WHEN** 在 Windows 8.1（6.3.9600）上执行 CLI 下载
- **THEN** 下载正常进行，`startup.log` 中出现 platform-warning 记录，终端不出现阻塞式提示

#### Scenario: 正常系统不产生多余记录
- **WHEN** 在 Windows 10 19045 上执行 CLI 下载
- **THEN** 不写 platform-warning 记录

### Requirement: 进度输出

CLI SHALL 在标准输出为终端时以单行刷新方式显示进度，基于完成百分比、速度与剩余时间。
输出被重定向时 SHALL 改为按完成度每 10% 打印一行。

`-q` 时 MUST 不输出进度，只输出最终文件路径或错误信息。

#### Scenario: 终端输出
- **WHEN** 用户在交互式终端运行
- **THEN** 进度在同一行刷新，完成后保留最终路径

#### Scenario: 静默模式
- **WHEN** 用户加 `-q`
- **THEN** 过程中没有进度输出，结束时 stderr 只有一行结果或错误

### Requirement: 中断与续传

收到 Ctrl+C 时，CLI MUST 暂停下载、把进度写入 journal，并以退出码 130 结束。

再次执行同一条命令时 MUST 从已有进度继续，而不是从头下载。

#### Scenario: 中断后继续
- **WHEN** 下载到 40% 时按 Ctrl+C，随后再次执行同一命令
- **THEN** 第二次从约 40% 处继续，最终文件字节完整

### Requirement: 退出码

CLI MUST 使用固定退出码：0 完成、1 下载失败、2 用法错误、130 用户中断。

#### Scenario: 下载失败
- **WHEN** 服务器返回 404
- **THEN** 退出码为 1，stderr 给出失败原因

### Requirement: 旧系统上的 GUI 引导

GUI 因系统版本过低被拒绝时，错误文案 MUST 提到同目录下的 `flashdown-cli.exe` 及其用法。

#### Scenario: 8.1 上双击 GUI
- **WHEN** 用户在 8.1 上双击 `flashdown.exe`
- **THEN** 弹窗中出现 `flashdown-cli.exe` 字样与一条示例命令

### Requirement: 发布形态

发布 zip MUST 同时包含 `flashdown.exe`、`flashdown-cli.exe` 与 `SYSTEM-REQUIREMENTS.txt`，
且说明文档 SHALL 区分两者的系统要求（GUI：Win10 1809+；CLI：尽力支持旧系统）。

#### Scenario: 解压发布包
- **WHEN** 用户解压 v0.3.0 及之后的 zip
- **THEN** 三个文件都在，`SYSTEM-REQUIREMENTS.txt` 写明两个入口的差异
