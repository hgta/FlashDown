## ADDED Requirements

### Requirement: Adaptive block partitioning
The system SHALL partition a download into blocks of adaptive size, computed as
`clamp(align64KB(totalSize / (maxJets × 8)), 256KB, 8MB)`, so that block count is roughly
8× the configured jet count. Blocks SHALL be the single unit of persistence, scheduling,
and progress visualization.

#### Scenario: Large file with 5 jets
- **WHEN** a 1.2 GB file is downloaded with `maxJets = 5`
- **THEN** the block size resolves to `1.2GB / 40 = 30MB` (clamped to 8MB) and the file is split into ~150 blocks

#### Scenario: Small file
- **WHEN** a 3 MB file is downloaded
- **THEN** the computed block size is clamped up to 256KB, producing ~12 blocks, and the system still opens multiple jets

### Requirement: Stateless jets consuming a shared block queue
The system SHALL run N concurrent jets (default 5, configurable up to 30). Each jet SHALL be
stateless: it acquires the block with the largest remaining bytes, downloads its
`[Start+Done, End)` range, releases the block, and repeats. A jet SHALL hold no state that
must be recovered after crash, pause, or reconfiguration.

#### Scenario: Fast jet picks up more work
- **WHEN** 5 jets run on 150 blocks and jet 3's connection is twice as fast as the others
- **THEN** jet 3 acquires and completes proportionally more blocks without any re-planning logic

#### Scenario: Jet count changed mid-download
- **WHEN** the user raises max jets from 5 to 10 while a task is downloading
- **THEN** the running task scales to 10 jets with no restart and no loss of completed bytes

### Requirement: Byte-level resumability from block state
The system SHALL persist each block as `{Start, End, Done}` and resume by issuing
`Range: bytes=(Start+Done)-(End-1)`. Resume granularity SHALL be byte-level, not block-level.
The system SHALL NOT require a separate piece bitmap.

#### Scenario: Process killed mid-download
- **WHEN** the application is killed at 62% of a download and relaunched
- **THEN** the task resumes from the persisted `Done` offsets, losing at most one journal flush interval of progress

#### Scenario: Resume with changed jet count
- **WHEN** a task paused at 40% is resumed with a different jet count
- **THEN** the block queue is rebuilt from persisted `Done` values and download continues correctly

### Requirement: Range capability detection with graceful degradation
Before starting, the system SHALL probe the resource for `Accept-Ranges` and total size.
If the server returns HTTP `200` instead of `206` for a ranged request, the system SHALL
degrade the whole task to a single connection downloading from byte 0, discard partial data,
and mark the task as "does not support resume".

#### Scenario: Server ignores Range header
- **WHEN** a server responds `200 OK` with the full body to a `bytes=1000-1999` request
- **THEN** the task restarts as single-connection from offset 0 and the UI shows a "no resume support" badge

#### Scenario: Server advertises range support
- **WHEN** a server responds `206 Partial Content` with `Content-Range`
- **THEN** the system parses the true total size from `Content-Range` and proceeds with multi-jet download

### Requirement: Content fingerprint verification before resume
The system SHALL record `ETag` and `Last-Modified` on first probe. Before resuming, the system
SHALL re-probe and compare; on mismatch it SHALL NOT silently continue but prompt the user to
choose between restarting the download and resuming anyway.

#### Scenario: Remote file changed during pause
- **WHEN** a task paused for 3 hours is resumed and the `ETag` differs from the recorded value
- **THEN** a dialog offers "Restart download" and "Continue anyway", defaulting to restart

### Requirement: Concurrent random writes into a single file
The system SHALL open one file handle per task and have all jets write via `File.WriteAt`
(pwrite semantics) at their own offsets. Writes SHALL NOT be serialized behind a mutex and
SHALL NOT corrupt each other. Each jet SHALL write through a bounded buffer (256KB) to prevent
unbounded memory growth.

#### Scenario: Five jets writing simultaneously
- **WHEN** 5 jets write to different byte ranges of the same 1.2 GB file concurrently
- **THEN** every byte written lands at its intended offset and the completed file is byte-identical to the source

### Requirement: Configurable preallocation strategy
The system SHALL support three preallocation policies — `auto`, `always`, `never`.

Preallocation via `Truncate` completes in under a millisecond regardless of size (measured:
4 GiB in 0 ms) but immediately reserves the full file size on disk (measured: 100% of the
requested size). **Its cost is disk space, not time.** Under `auto` the system SHALL
preallocate only when the target volume is NTFS AND free space is at least 1.2× the expected
file size, and SHALL otherwise grow the file on demand.

#### Scenario: Plenty of free space
- **WHEN** a 4 GB file is downloaded to NTFS with 60 GB free and policy `auto`
- **THEN** the file is preallocated instantly and the final size is visible immediately

#### Scenario: Not enough free space
- **WHEN** a 20 GB file is downloaded with policy `auto` to a volume with 15 GB free
- **THEN** the system does NOT preallocate, grows the file on demand, and the download proceeds normally instead of failing at the start

#### Scenario: Non-NTFS volume
- **WHEN** the target volume is exFAT or FAT32
- **THEN** `auto` skips preallocation regardless of free space

#### Scenario: Explicit `always` policy with insufficient space
- **WHEN** policy is `always` and free space is less than the file size
- **THEN** the task fails fast with a clear "insufficient disk space" message rather than partially filling the volume

### Requirement: Atomic journal persistence
The system SHALL persist task state to `<name>.fd!.state` using atomic replacement
(write `.tmp` → `Sync` → rename). State SHALL be flushed every 1–3 seconds and whenever a
block completes. A corrupted or unreadable state file SHALL be backed up and the task SHALL
fall back to a fresh download rather than crashing.

#### Scenario: State file truncated by power loss
- **WHEN** the application restarts and finds a `.fd!.state` that fails JSON parsing
- **THEN** the corrupt file is renamed to `.fd!.state.bak`, and the task restarts from zero with a user-visible warning

### Requirement: Stall detection and classified retry
The system SHALL treat "no bytes received for 60 seconds" as a failure and retry. Errors SHALL
be classified as retryable (network reset, timeout, 5xx, 429) or terminal (404, 410, auth
failure). Retryable errors SHALL honor a configurable max attempt count and backoff interval;
terminal errors SHALL stop the task immediately.

#### Scenario: Connection stalls
- **WHEN** a jet receives no data for 60 seconds
- **THEN** the jet aborts, returns its block to the queue with `Done` preserved, and retries after the backoff interval

#### Scenario: Resource removed upstream
- **WHEN** the server returns 404 during a retry
- **THEN** the task transitions to `Error` immediately without consuming further retry attempts

### Requirement: Engine independence from UI
The `internal/engine` package SHALL NOT import any Wails, windowing, or frontend code. The
engine SHALL be runnable headless and fully unit-testable, exposing progress through a
callback/event interface rather than direct UI calls.

#### Scenario: Engine unit test without a window
- **WHEN** a test constructs a manager, adds a task against a local HTTP server, and waits
- **THEN** the file is downloaded and verified with no windowing runtime initialized
