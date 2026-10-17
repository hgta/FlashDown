## ADDED Requirements

### Requirement: Classic FlashGet layout regions
The main window SHALL consist of six regions, matching the classic layout: menu bar, toolbar,
left category tree, task grid, bottom detail pane, and status bar. The category tree and detail
pane SHALL be resizable via splitters, and their sizes SHALL persist across restarts.

#### Scenario: Adjusting the detail pane
- **WHEN** the user drags the splitter to enlarge the bottom pane and restarts the app
- **THEN** the bottom pane reopens at the same height

### Requirement: Task grid columns
The task grid SHALL display, in FlashGet order: filename, size, progress (with an in-cell
progress bar and percentage), elapsed time, remaining time, speed, and block/jet count.
Columns SHALL be sortable and their visibility and order SHALL be configurable.

#### Scenario: Sorting by speed
- **WHEN** the user clicks the speed column header
- **THEN** tasks reorder by current speed, descending, and the sort persists for the session

#### Scenario: Hiding a column
- **WHEN** the user hides the elapsed-time column via the header context menu
- **THEN** the column disappears and the preference persists across restarts

### Requirement: Bottom detail pane with three tabs
The bottom pane SHALL provide three tabs — 块图 (block map), Jet (per-connection list), and
日志 (log). Content SHALL follow the currently selected task.

#### Scenario: Switching tasks
- **WHEN** the user selects a different task in the grid
- **THEN** all three tabs re-render for the newly selected task without a visible flicker

### Requirement: Block map visualization
The 块图 tab SHALL render the file as a grid of roughly 300 cells, colored by state (completed,
downloading per-jet with distinct hues, pending). It SHALL be rendered on `<canvas>`, not as
hundreds of DOM nodes, and SHALL refresh at 2–4 fps.

#### Scenario: Watching blocks fill
- **WHEN** a multi-jet download is running
- **THEN** cells progressively fill with the color of the jet that downloaded them, and the UI remains responsive

### Requirement: Jet list visualization
The Jet tab SHALL show one row per active connection with its byte range, a progress bar,
percentage, and current speed, updating at 2–4 fps.

#### Scenario: Jet completes and takes a new block
- **WHEN** a jet finishes its block and acquires another
- **THEN** its row updates to the new range and resets its progress bar

### Requirement: Status bar
The status bar SHALL show total task count, count currently downloading, aggregate speed, and
active jet count.

#### Scenario: Idle state
- **WHEN** no task is downloading
- **THEN** the status bar shows 0 downloading and 0 B/s without stale values from previous sessions

### Requirement: Menu bar and toolbar
The window SHALL provide a menu bar (文件 / 编辑 / 查看 / 任务 / 工具 / 帮助) and a toolbar with
at least: 新建, 添加成批任务, 开始, 暂停, 删除, 属性, 目录, 查找, 上移, 下移, 选项.

#### Scenario: Toolbar action on selection
- **WHEN** the user selects 3 tasks and clicks 暂停
- **THEN** all 3 tasks pause and the remaining tasks are unaffected

### Requirement: Modern rendering with light and dark themes
Layout and information density SHALL replicate FlashGet, but rendering SHALL use modern spacing,
CSS-variable-driven theming with light and dark variants following the OS preference, and SHALL
be crisp on high-DPI displays at 125%, 150%, and 200% scaling.

#### Scenario: OS theme switch
- **WHEN** the user switches Windows from light to dark while the app is running
- **THEN** the UI follows without requiring a restart

#### Scenario: High-DPI display
- **WHEN** the app runs on a 200%-scaled display
- **THEN** text and icons render sharply with no clipping in the grid or toolbar

### Requirement: Virtualized grid for large task lists
The task grid SHALL virtualize rows so that lists of 1000+ tasks remain scrollable and
responsive while downloads are active.

#### Scenario: Thousand tasks downloading
- **WHEN** 1000 tasks exist and 3 are actively downloading
- **THEN** scrolling stays smooth and updates do not drop frames
