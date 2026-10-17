## ADDED Requirements

### Requirement: Global speed limit
The system SHALL enforce a configurable global download speed ceiling shared by all running
tasks. The limit SHALL be adjustable at runtime without restarting active downloads.

#### Scenario: Global cap reached
- **WHEN** the global limit is 1 MB/s and three tasks together request 3 MB/s
- **THEN** aggregate throughput converges to 1 MB/s and is distributed across active tasks

#### Scenario: Limit changed while downloading
- **WHEN** the user lowers the global limit from unlimited to 500 KB/s mid-download
- **THEN** throughput drops to the new ceiling within one token bucket window (~100ms) with no connection drop

### Requirement: Per-task speed limit
The system SHALL support a per-task speed ceiling that overrides or narrows the global limit.
The effective rate for a jet SHALL be the more restrictive of the global and task buckets.

#### Scenario: Task capped below global
- **WHEN** global is unlimited and task A is capped at 200 KB/s
- **THEN** task A downloads at 200 KB/s while other tasks remain unaffected

### Requirement: Time-schedule based limit
The system SHALL support time-range rules mapping a clock interval to a speed ceiling
(for example 09:00–18:00 → 200 KB/s). Rules SHALL be evaluated against local time and
re-evaluated on rule boundaries without restarting downloads.

#### Scenario: Entering a restricted window
- **WHEN** a rule caps speed to 200 KB/s between 09:00 and 18:00 and the clock crosses 09:00
- **THEN** active downloads throttle to 200 KB/s without dropping connections

### Requirement: Token bucket throttling with bounded latency
Throttling SHALL use a token bucket with burst capped at `rate × 100ms`. Consumption SHALL
occur after reading bytes (post-read accounting). A single wait SHALL never exceed 100ms, so
low-rate limits do not stall the jet loop.

#### Scenario: Very low rate limit
- **WHEN** a task is limited to 10 KB/s
- **THEN** the jet loop still wakes at least every 100ms and the UI keeps receiving progress updates

### Requirement: Sliding-window speed metering
The system SHALL compute current speed over a sliding window (approximately 5 seconds) rather
than an instantaneous sample, and SHALL emit a smoothed value suitable for display and graphing.

#### Scenario: Bursty transfer
- **WHEN** a connection alternates between 2 MB/s bursts and idle gaps
- **THEN** the displayed speed changes smoothly instead of flickering between 0 and 2 MB/s

### Requirement: Remaining time estimation
The system SHALL estimate remaining time from the smoothed speed and remaining bytes, and SHALL
display "unknown" rather than a misleading value when speed is effectively zero or the total
size is unknown.

#### Scenario: Stalled task
- **WHEN** a task has received no bytes for 10 seconds
- **THEN** the remaining-time column shows "未知" instead of an ever-growing duration
