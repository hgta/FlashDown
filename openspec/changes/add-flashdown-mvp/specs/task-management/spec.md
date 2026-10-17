## ADDED Requirements

### Requirement: New task dialog
The system SHALL provide a new-task dialog capturing: URL, save-as filename, save directory
(with category selection), block/jet count, referer, credentials, comment, and start mode
(immediate / manual / queued). The dialog SHALL be reachable from the toolbar, the menu, and
by dropping a link onto the main window or floating window.

#### Scenario: Creating a task with referer for a hotlink-protected host
- **WHEN** the user pastes a URL and fills the referer field with the source page
- **THEN** the request carries the `Referer` header and the server returns `206` instead of `403`

#### Scenario: Manual start mode
- **WHEN** the user selects start mode "manual"
- **THEN** the task is created in `Paused` state and does not consume a concurrency slot

### Requirement: Task state machine
The system SHALL model each task with the states `Draft`, `Waiting`, `Probing`, `Downloading`,
`Paused`, `Verifying`, `Completing`, `Finished`, `Error`, `Removed`. Transitions SHALL be
explicit and persisted so that a restart restores the correct state.

#### Scenario: Queue is full
- **WHEN** 3 tasks are downloading and the concurrency cap is 3, and a 4th task is started
- **THEN** the 4th task enters `Waiting` and begins automatically when a slot frees

#### Scenario: Pause and resume
- **WHEN** the user pauses a downloading task and resumes it an hour later
- **THEN** all jets stop, block `Done` offsets are journaled, and the download continues from those offsets

### Requirement: Concurrency control
The system SHALL enforce a configurable cap on simultaneously downloading tasks (default 3)
and a per-task cap on jets (default 5, max 30). Waiting tasks SHALL be started in queue order.

#### Scenario: Completing a task frees a slot
- **WHEN** one of 3 active tasks finishes
- **THEN** the highest-priority `Waiting` task transitions to `Probing` automatically

### Requirement: Delete and recycle
Deleting a task SHALL move it to a "已删除" (trash) category node. The system SHALL offer
"delete task only" (keep file) and "delete task and file". Purging from trash SHALL permanently
remove the entry.

#### Scenario: Accidental deletion recovery
- **WHEN** the user deletes a finished task
- **THEN** the task appears under 已删除 and can be restored, with the already-downloaded file left intact

### Requirement: Completion handling
On completion the system SHALL verify the final byte count against the expected total, rename
`<name>.fd!` to the final filename, move the file into its category directory, and remove the
state file. Rename collisions SHALL be resolved by a numeric suffix.

#### Scenario: Target filename already exists
- **WHEN** `setup.zip` already exists in the destination directory at completion time
- **THEN** the downloaded file is written as `setup(1).zip` and the task records the final path

#### Scenario: Antivirus holds the file during rename
- **WHEN** the rename fails because another process holds the handle
- **THEN** the system retries with backoff and reports a clear error if it ultimately fails

### Requirement: Retry policy configuration
The system SHALL expose configurable max retry attempts and backoff interval, and SHALL surface
the current retry count per task in the UI.

#### Scenario: Repeated network failures
- **WHEN** a task fails 5 consecutive times at the configured max
- **THEN** the task moves to `Error` with a resume action available, preserving all downloaded bytes

### Requirement: Task ordering and bulk actions
The system SHALL support reordering waiting tasks (move up / move down) and bulk actions
(start all / pause all) from the toolbar.

#### Scenario: Prioritizing a task
- **WHEN** the user moves a waiting task to the top of the queue
- **THEN** it is the next task started when a concurrency slot frees
