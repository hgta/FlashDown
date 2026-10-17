/**
 * Hand-written mirrors of the Go DTOs in app/events.go and internal/config.
 *
 * The generated bindings under frontend/bindings are plain .js with @ts-check
 * comments, so they carry no usable type information. Declaring the shapes here
 * keeps the UI fully typed and turns a Go/TS drift into a compile error at the
 * boundary instead of a runtime surprise.
 */

export type TaskState =
  | 'draft'
  | 'waiting'
  | 'probing'
  | 'downloading'
  | 'paused'
  | 'verifying'
  | 'completing'
  | 'finished'
  | 'error'
  | 'removed'

export interface TaskDTO {
  id: string
  name: string
  url: string
  category: string
  state: TaskState
  size: number
  done: number
  speed: number
  elapsedMs: number
  remainMs: number
  jets: number
  blocks: number
  resumable: boolean
  comment: string
  error: string
}

export interface Snapshot {
  tasks: TaskDTO[]
  downloading: number
  totalSpeed: number
  activeJets: number
  atMillis: number
}

export interface BlockDTO {
  start: number
  end: number
  done: number
  /** -1 when the block has not been claimed by a jet yet. */
  jet: number
}

export interface JetDTO {
  index: number
  start: number
  end: number
  done: number
  speed: number
}

export interface TaskDetail {
  taskId: string
  blocks: BlockDTO[]
  jets: JetDTO[]
  log: string[]
}

export interface SpeedTick {
  speed: number
  history: number[]
  atMillis: number
}

export interface Category {
  id: string
  name: string
  dir: string
  fileTypes: string
  builtIn: boolean
}

export interface Rule {
  patterns: string
  category: string
  enabled: boolean
}

export interface RetryConfig {
  maxAttempts: number
  backoffSeconds: number
}

export interface LimitsConfig {
  globalBytesPerSec: number
  scheduleEnabled: boolean
  scheduleFrom: string
  scheduleTo: string
  scheduleBytesPerSec: number
}

export interface UIConfig {
  theme: 'system' | 'light' | 'dark'
  floatWindow: boolean
  notifyOnFinish: boolean
  clipboardWatch: boolean
  clipboardAutoAdd: boolean
  columns: string[]
  treeWidthRatio: number
  detailHeightRatio: number
}

export interface AppConfig {
  version: number
  downloadRoot: string
  maxConcurrentTasks: number
  maxJets: number
  preallocate: 'auto' | 'always' | 'never'
  categories: Category[]
  rules: Rule[]
  retry: RetryConfig
  limits: LimitsConfig
  ui: UIConfig
}

export interface StartupInfo {
  config: AppConfig
  snapshot: Snapshot
  warning: string
  configPath: string
  version: string
}

/** The left tree's fixed status nodes, followed by the user's categories. */
export type FilterKind = 'status' | 'category'

export interface TreeFilter {
  kind: FilterKind
  id: string
}
