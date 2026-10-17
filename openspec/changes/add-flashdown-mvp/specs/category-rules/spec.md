## ADDED Requirements

### Requirement: Category tree with status nodes and categories
The system SHALL render a left-hand tree with three fixed status nodes — 正在下载, 已完成,
已删除 — followed by user categories. Selecting a node SHALL filter the task grid to that set.
Each status node SHALL display its task count.

#### Scenario: Filtering by category
- **WHEN** the user selects the 软件 category
- **THEN** the task grid shows only tasks assigned to 软件, regardless of their state

#### Scenario: Selecting a status node
- **WHEN** the user selects 正在下载
- **THEN** the grid shows all tasks currently in `Downloading` or `Waiting`, across all categories

### Requirement: Default categories
On first launch the system SHALL create four default categories — 软件, 游戏, 驱动程序, MP3 —
each with its own default subdirectory under the configured download root.

#### Scenario: First launch
- **WHEN** the application runs for the first time
- **THEN** the tree contains 软件 / 游戏 / 驱动程序 / MP3 and each has a directory on disk

### Requirement: Per-category download directory
Each category SHALL own a target directory. A task assigned to a category SHALL have its
completed file moved into that directory. The system SHALL create the directory if missing.

#### Scenario: Category directory missing at completion
- **WHEN** a task completes and its category directory was deleted meanwhile
- **THEN** the directory is recreated and the file is moved successfully

### Requirement: Category management
The system SHALL allow creating, renaming, and deleting user categories and changing their
directories. Deleting a category SHALL reassign or unassign its tasks rather than deleting files.

#### Scenario: Deleting a category with tasks
- **WHEN** the user deletes 游戏 which contains 3 completed tasks
- **THEN** those tasks move to the unassigned/default category and their files remain on disk

### Requirement: Download rules by file type
The system SHALL support rules mapping a file-type pattern to a category, for example
`*.zip;*.rar;*.exe` → 软件. When a new task is created whose URL or filename matches a rule,
the system SHALL pre-select that category automatically. Rules SHALL be ordered, with the first
match winning.

#### Scenario: Auto-classified download
- **WHEN** the user adds `https://host/game/wow.iso` and a rule maps `*.iso` → 游戏
- **THEN** the new-task dialog opens with 游戏 pre-selected and the game directory as target

#### Scenario: No rule matches
- **WHEN** no rule matches `report.pdf`
- **THEN** the task falls back to the default category defined in settings

### Requirement: Rule management UI
The system SHALL provide a settings screen to add, edit, reorder, enable/disable, and delete
download rules.

#### Scenario: Disabling a rule temporarily
- **WHEN** the user disables the `*.iso` → 游戏 rule
- **THEN** new `.iso` downloads fall back to the default category while existing assignments are untouched
