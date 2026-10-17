## ADDED Requirements

### Requirement: Frameless always-on-top floating window
The system SHALL provide a secondary window, frameless and always on top, excluded from the
taskbar, with a default size around 220×90 px. It SHALL be independently showable/hideable
without affecting the main window.

#### Scenario: Floating window over a fullscreen app
- **WHEN** the user enables the floating window and switches to a fullscreen application
- **THEN** the floating window remains visible on top

#### Scenario: Hiding the floating window only
- **WHEN** the user hides the floating window from its context menu
- **THEN** the main window and all running downloads are unaffected

### Requirement: Draggable and position-persistent
The floating window SHALL be draggable by its body (no title bar) and SHALL remember its screen
position across restarts.

#### Scenario: Repositioning
- **WHEN** the user drags the window to the bottom-right corner and restarts the app
- **THEN** it reappears at the same coordinates, clamped back on-screen if the display changed

### Requirement: Live speed display and graph
The floating window SHALL show the current aggregate speed and a rolling graph of the last
60 samples (about one minute), fed by the same `speed:tick` event as the main window with no
additional IPC cost.

#### Scenario: Speed changes
- **WHEN** downloads start and stop
- **THEN** the graph scrolls smoothly and returns to zero when nothing is downloading

### Requirement: Drop basket for links
The floating window SHALL accept dragged text and links; dropping a URL SHALL open the new-task
dialog pre-filled with that URL. Dropping onto the main window or grid SHALL behave identically.

#### Scenario: Dragging a link from a browser
- **WHEN** the user drags a URL from the browser address bar onto the floating window
- **THEN** the new-task dialog opens with the URL already filled

#### Scenario: Dropping non-URL text
- **WHEN** the user drops plain text that is not a URL
- **THEN** nothing is created and a brief inline hint explains that a link is required

### Requirement: Right-click context menu
Because the window is frameless, the system SHALL render its own context menu on right-click
with at least: 显示/隐藏主窗口, 新建任务, 全部开始, 全部暂停, 悬浮窗设置, 退出.

#### Scenario: Quick pause from the floating window
- **WHEN** the user right-clicks the floating window and chooses 全部暂停
- **THEN** all downloading tasks pause without opening the main window
