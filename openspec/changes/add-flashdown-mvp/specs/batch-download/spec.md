## ADDED Requirements

### Requirement: FlashGet-style wildcard syntax
The system SHALL support batch task creation from a URL template where the varying portion is
wrapped in parentheses containing `(*)`, plus a configurable wildcard length, start value, and
end value — matching classic FlashGet's 添加成批任务 dialog.

#### Scenario: Numeric sequence with zero padding
- **WHEN** the template is `http://host/pic(*).jpg` with wildcard length 2, from `01` to `20`
- **THEN** 20 tasks are generated as `pic01.jpg` through `pic20.jpg`, preserving zero padding

#### Scenario: Sequence without padding
- **WHEN** the template is `http://host/p(*).zip` with length 1, from `1` to `10`
- **THEN** 10 tasks are generated from `p1.zip` through `p10.zip`

### Requirement: Range and step syntax
The system SHALL additionally support explicit range syntax `(1-10)` and stepped syntax
`(1-20:2)`, producing inclusive sequences with the given step.

#### Scenario: Stepped range
- **WHEN** the template is `http://host/part(1-20:2).rar`
- **THEN** 10 tasks are generated for parts 1, 3, 5, … 19

### Requirement: Alphabetical sequence
The system SHALL support letter ranges such as `(a-z)` and `(A-Z)`, generating the inclusive
alphabetic sequence.

#### Scenario: Lowercase letter range
- **WHEN** the template is `http://host/ch(a-d).pdf`
- **THEN** 4 tasks are generated for `cha.pdf` through `chd.pdf`

### Requirement: Multiple wildcards with cartesian expansion
The system SHALL support more than one parenthesized group in a single template and SHALL expand
them as a cartesian product, left to right.

#### Scenario: Two groups
- **WHEN** the template is `http://host/s(1-2)/img(01-03).png`
- **THEN** 6 tasks are generated covering every combination of season and image index

### Requirement: Preview before confirmation
The system SHALL show a preview list of all generated URLs with a count before creating tasks,
and SHALL allow deselecting individual entries. Nothing SHALL be queued until the user confirms.

#### Scenario: Previewing a large batch
- **WHEN** a template expands to 500 URLs
- **THEN** the preview lists all 500 with checkboxes, and no task is created until confirmation

#### Scenario: Deselecting entries
- **WHEN** the user unchecks 3 of 20 previewed URLs and confirms
- **THEN** exactly 17 tasks are created

### Requirement: Shared settings across a batch
All tasks generated from one batch SHALL share the same category, jet count, start mode, and
save directory unless the user overrides them on the confirmation dialog.

#### Scenario: Batch to a single category
- **WHEN** the user creates a 30-file batch assigned to 图片
- **THEN** all 30 tasks land in 图片 and complete into the 图片 directory
