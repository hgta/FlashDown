## ADDED Requirements

### Requirement: Download history records
The system SHALL append a record for every finished task to `history.jsonl`, capturing final
filename, URL, size, category, completion timestamp, and final path. Records SHALL be loaded
into an in-memory index at startup.

#### Scenario: Restarting after many downloads
- **WHEN** the app restarts with 5000 history records
- **THEN** the index is built in under one second and the UI becomes interactive

### Requirement: Search across history
The system SHALL provide a search that matches filename and URL across both active tasks and
history, with results indicating which set each hit came from.

#### Scenario: Searching by filename fragment
- **WHEN** the user searches `ubuntu`
- **THEN** all matching active tasks and history entries are listed with their state and completion date

### Requirement: Already-downloaded hint
When a new task's URL matches a history record, the system SHALL warn that the file was already
downloaded, show the previous path and date, and let the user continue or cancel.

#### Scenario: Re-adding a known download
- **WHEN** the user adds a URL matching a history entry from 3 months ago
- **THEN** a提示 shows the earlier path and date, offering 仍然下载 or 取消

### Requirement: Per-task log pane
Each task SHALL maintain a chronological log of state transitions, errors, retries, and HTTP
responses, viewable in the 日志 tab of the detail pane.

#### Scenario: Investigating repeated retries
- **WHEN** a task has retried 4 times
- **THEN** the 日志 tab lists each attempt with timestamp, HTTP status, and error reason

### Requirement: Export and import unfinished tasks
The system SHALL export the list of unfinished tasks (URL, save path, category, state file path)
to a portable file, and SHALL import such a file to re-register tasks on another machine or after
reinstalling.

#### Scenario: Migrating to a new machine
- **WHEN** the user exports unfinished tasks, copies the data files, and imports on another PC
- **THEN** the tasks reappear in their original categories and resume from their saved progress

#### Scenario: Importing with missing data files
- **WHEN** an imported task references a `.fd!` data file that does not exist
- **THEN** the task is registered as needing a fresh download and the user is told which entries were affected

### Requirement: Export URL lists
The system SHALL export selected URLs as plain text, one per line, for use in other tools.

#### Scenario: Exporting a batch
- **WHEN** the user selects 20 tasks and chooses export URLs
- **THEN** a text file with 20 lines is produced in the chosen location
