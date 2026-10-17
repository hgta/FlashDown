## ADDED Requirements

### Requirement: Tray icon with theme-aware rendering
The system SHALL place an icon in the Windows notification area that remains visible while the
application runs, and SHALL supply distinct light and dark variants so it stays legible under
both taskbar themes.

#### Scenario: Switching taskbar theme
- **WHEN** the user switches Windows between light and dark taskbar themes
- **THEN** the tray icon switches to the matching variant automatically

### Requirement: Closing the main window hides to tray
Closing the main window SHALL hide it and keep downloads running in the background rather than
exiting. Exiting SHALL require an explicit action from the tray menu or the floating window menu.

#### Scenario: Closing the window during a download
- **WHEN** the user closes the main window while two tasks are downloading
- **THEN** both tasks keep downloading and the tray icon remains available

#### Scenario: Explicit exit
- **WHEN** the user chooses 退出 from the tray menu with tasks still downloading
- **THEN** the application journals all task state and exits cleanly

### Requirement: Tray context menu
The tray icon SHALL expose a context menu with at least: 显示/隐藏主窗口, 新建任务, 全部开始,
全部暂停, 打开下载目录, 退出.

#### Scenario: Restoring a hidden window
- **WHEN** the main window is hidden and the user chooses 显示主窗口 from the tray
- **THEN** the window is restored, focused, and raised above other windows

### Requirement: Completion notification
The system SHALL raise a notification when a task completes while the main window is hidden or
unfocused. Notifications SHALL be suppressible in settings.

#### Scenario: Task finishes in the background
- **WHEN** a download completes while the main window is hidden
- **THEN** a notification reports the filename and offers an action to open its folder

#### Scenario: Notifications disabled
- **WHEN** the user disables notifications in settings
- **THEN** no notification is raised, but the task still completes and is listed normally
