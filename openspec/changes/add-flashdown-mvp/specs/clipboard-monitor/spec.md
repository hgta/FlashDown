## ADDED Requirements

### Requirement: Clipboard link monitoring
The system SHALL monitor the system clipboard while enabled, detect `http://` and `https://`
text, and offer to create a download task for it.

#### Scenario: Copying a download link
- **WHEN** the user copies `https://host/file.zip` and monitoring is enabled
- **THEN** the new-task dialog opens with that URL pre-filled

#### Scenario: Copying non-link text
- **WHEN** the user copies ordinary prose
- **THEN** nothing happens and no dialog appears

### Requirement: Duplicate and self-copy suppression
The system SHALL ignore clipboard content that equals the previously handled value, and SHALL
ignore URLs already present in the active or finished task list unless the user explicitly
re-adds them.

#### Scenario: Repeated identical copy
- **WHEN** the same URL is copied twice in a row
- **THEN** only one prompt is raised

#### Scenario: Copying an already-downloaded URL
- **WHEN** the user copies a URL already recorded in download history
- **THEN** no prompt is raised; the existing history entry is surfaced instead if the user searches for it

### Requirement: Toggle and auto-add mode
Monitoring SHALL be disableable in settings. When auto-add mode is enabled, detected links SHALL
be queued directly using the default category and settings instead of opening a dialog.

#### Scenario: Auto-add enabled
- **WHEN** monitoring and auto-add are both enabled and a link is copied
- **THEN** the task is created and starts immediately with default settings, with no dialog

#### Scenario: Monitoring disabled
- **WHEN** clipboard monitoring is turned off
- **THEN** copying links never raises prompts or creates tasks
