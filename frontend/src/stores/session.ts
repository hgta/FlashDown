/**
 * Central UI state.
 *
 * Deliberately a plain reactive module rather than Pinia - there is exactly one
 * session and adding a store library would buy nothing. Every window (main and
 * floating) subscribes to the same three broadcast events, which is why the
 * floating window's speed readout costs no extra round trip.
 */

import { computed, reactive } from 'vue'
import { Events } from '@wailsio/runtime'
import { Service } from '../../bindings/github.com/hgta/flashdown/app'
import type {
  Snapshot as WireSnapshot,
  TaskDTO as WireTaskDTO,
  TaskDetail as WireTaskDetail,
} from '../../bindings/github.com/hgta/flashdown/app'
import type { Config as WireConfig } from '../../bindings/github.com/hgta/flashdown/internal/config'
import { applyTheme } from '../theme/theme'
import type {
  AppConfig,
  Snapshot,
  TaskDTO,
  TaskDetail,
  TaskState,
  TreeFilter,
  UIConfig,
} from '../types'

/** The three fixed nodes at the top of the tree, in FlashGet order. */
export const STATUS_NODES = [
  { id: 'downloading', name: '正在下载' },
  { id: 'finished', name: '已完成' },
  { id: 'removed', name: '已删除' },
] as const

export const STATE_LABELS: Record<string, string> = {
  draft: '草稿',
  waiting: '等待',
  probing: '探测',
  downloading: '下载中',
  paused: '已暂停',
  verifying: '校验中',
  completing: '收尾中',
  finished: '已完成',
  error: '出错',
  removed: '已删除',
}

/** Column definitions. Order comes from config.ui.columns. */
export const COLUMNS: Record<string, { title: string; width: number; align?: 'right' }> = {
  name: { title: '文件名', width: 0 },
  size: { title: '大小', width: 78, align: 'right' },
  progress: { title: '完成度', width: 146 },
  elapsed: { title: '用时', width: 62, align: 'right' },
  remaining: { title: '剩余', width: 62, align: 'right' },
  speed: { title: '速度', width: 82, align: 'right' },
  blocks: { title: '分块', width: 52, align: 'right' },
}

function emptySnapshot(): Snapshot {
  return { tasks: [], downloading: 0, totalSpeed: 0, activeJets: 0, atMillis: 0 }
}

/*
 * Wire-to-UI adapters.
 *
 * The generated bindings describe the wire faithfully, which means two things
 * the UI does not want to know about: Go slices arrive as `T[] | null`, and Go
 * string constants arrive as plain `string`. Conforming once, here, is what lets
 * types.ts stay the strict contract the components are written against - and it
 * means a nil slice can never reach the grid.
 */

const THEMES: readonly UIConfig['theme'][] = ['system', 'light', 'dark']
const PREALLOCATE: readonly AppConfig['preallocate'][] = ['auto', 'always', 'never']

function pick<T extends string>(allowed: readonly T[], value: string, fallback: T): T {
  return (allowed as readonly string[]).includes(value) ? (value as T) : fallback
}

function toTask(wire: WireTaskDTO): TaskDTO {
  return { ...wire, state: wire.state as TaskState }
}

function toSnapshot(wire: WireSnapshot | null | undefined): Snapshot {
  return {
    tasks: (wire?.tasks ?? []).map(toTask),
    downloading: wire?.downloading ?? 0,
    totalSpeed: wire?.totalSpeed ?? 0,
    activeJets: wire?.activeJets ?? 0,
    atMillis: wire?.atMillis ?? 0,
  }
}

function toDetail(wire: WireTaskDetail | null | undefined): TaskDetail {
  return {
    taskId: wire?.taskId ?? '',
    blocks: wire?.blocks ?? [],
    jets: wire?.jets ?? [],
    log: wire?.log ?? [],
  }
}

function toConfig(wire: WireConfig): AppConfig {
  return {
    ...wire,
    preallocate: pick(PREALLOCATE, wire.preallocate, 'auto'),
    categories: wire.categories ?? [],
    rules: wire.rules ?? [],
    ui: {
      ...wire.ui,
      theme: pick(THEMES, wire.ui.theme, 'system'),
      columns: wire.ui.columns ?? [],
    },
  }
}

export const state = reactive({
  ready: false,
  version: '',
  configPath: '',
  warning: '',
  config: null as AppConfig | null,
  snapshot: emptySnapshot(),
  detail: null as TaskDetail | null,
  speed: 0,
  speedHistory: [] as number[],
  selectedId: '',
  /** Default to the active node, matching FlashGet's startup behaviour. */
  filter: { kind: 'status', id: 'downloading' } as TreeFilter,
  sortKey: 'name',
  sortDir: 1 as 1 | -1,
  busy: false,
  lastError: '',

  /**
   * Transient message for the drop baskets. Each window has its own JS context, so
   * a hint belongs to the window that was dropped on, which is what we want.
   */
  dropHint: '',

  /** The new-task dialog. It lives in the store so a drop or a menu can open it. */
  dialogOpen: false,
  dialogUrl: '',
  dialogError: '',
  dialogBusy: false,
})

/** Loads the initial payload and subscribes to the throttled broadcasts. */
export async function init(): Promise<void> {
  try {
    const info = await Service.GetStartupInfo()
    state.config = toConfig(info.config)
    state.snapshot = toSnapshot(info.snapshot)
    state.warning = info.warning ?? ''
    state.configPath = info.configPath ?? ''
    state.version = info.version ?? ''
    applyTheme(state.config.ui.theme)
    state.ready = true
  } catch (err) {
    state.lastError = `启动信息读取失败：${String(err)}`
    state.ready = true
    return
  }

  Events.On('tasks:snapshot', (event) => {
    if (event?.data) state.snapshot = toSnapshot(event.data)
  })

  Events.On('task:detail', (event) => {
    const detail = event?.data
    // Ignore detail that raced ahead of a selection change.
    if (detail && detail.taskId === state.selectedId) {
      state.detail = toDetail(detail)
    }
  })

  Events.On('speed:tick', (event) => {
    const tick = event?.data
    if (!tick) return
    state.speed = tick.speed
    state.speedHistory = tick.history ?? []
  })

  // OS file drops are delivered to Go, not the DOM (design.md D12); Go forwards
  // them here so the basket can acknowledge an Explorer drag.
  Events.On('files:dropped', (event) => {
    dropFiles(event?.data?.files ?? [])
  })

  // The floating window has no dialog of its own, so its drops travel through Go
  // and come back as this event on the main window.
  Events.On('dialog:new-task', (event) => {
    openTaskDialog(event?.data?.url ?? '')
  })
}

/**
 * Which node a task belongs to. Deliberately total: every task lands in exactly
 * one of the three fixed nodes, so nothing can become invisible.
 */
export function bucketOf(task: TaskDTO): 'downloading' | 'finished' | 'removed' {
  if (task.state === 'finished') return 'finished'
  if (task.state === 'removed') return 'removed'
  return 'downloading'
}

function matchesFilter(task: TaskDTO): boolean {
  if (state.filter.kind === 'status') return bucketOf(task) === state.filter.id
  return task.category === state.filter.id
}

function sortValue(task: TaskDTO): string | number {
  switch (state.sortKey) {
    case 'size':
      return task.size
    case 'progress':
      return task.size > 0 ? task.done / task.size : 0
    case 'elapsed':
      return task.elapsedMs
    case 'remaining':
      return task.remainMs
    case 'speed':
      return task.speed
    case 'blocks':
      return task.blocks
    case 'category':
      return task.category
    default:
      return task.name.toLowerCase()
  }
}

/** Rows for the grid: filtered, then sorted, then stable. */
export const visibleTasks = computed(() => {
  const rows = state.snapshot.tasks.filter(matchesFilter)
  rows.sort((a, b) => {
    const av = sortValue(a)
    const bv = sortValue(b)
    if (av === bv) return a.name.localeCompare(b.name)
    return av > bv ? state.sortDir : -state.sortDir
  })
  return rows
})

/** Task counts per fixed node, for the tree badges. */
export const counts = computed(() => {
  const out: Record<string, number> = { downloading: 0, finished: 0, removed: 0 }
  for (const task of state.snapshot.tasks) out[bucketOf(task)] += 1
  return out
})

/** Task counts per category id. */
export const categoryCounts = computed(() => {
  const out: Record<string, number> = {}
  for (const task of state.snapshot.tasks) {
    out[task.category] = (out[task.category] ?? 0) + 1
  }
  return out
})

export const selectedTask = computed(
  () => state.snapshot.tasks.find((t) => t.id === state.selectedId) ?? null,
)

/**
 * Whether the selected task can be started or paused.
 *
 * Shared by the toolbar and the task menu so the two can never disagree about
 * it; an errored task is startable because that is how a retry happens.
 */
export const canStartSelected = computed(() => {
  const task = selectedTask.value
  if (!task) return false
  return task.state === 'paused' || task.state === 'error' || task.state === 'draft'
})

export const canPauseSelected = computed(() => {
  const task = selectedTask.value
  if (!task) return false
  return task.state === 'downloading' || task.state === 'probing' || task.state === 'waiting'
})

/** Connection count in effect. Changing it applies to running tasks at once. */
export const maxJets = computed(() => state.config?.maxJets ?? 5)

/** The menu offers these; the engine clamps to 1..30 regardless. */
export const JET_CHOICES = [1, 3, 5, 8, 16, 30] as const

export async function setMaxJets(n: number): Promise<void> {
  if (!state.config || state.config.maxJets === n) return
  try {
    const applied = await Service.SetMaxJets(n)
    state.config.maxJets = applied
  } catch (err) {
    state.lastError = `设置连接数失败：${String(err)}`
  }
}

/** The columns actually shown, resolved from config so the order is user-driven. */
export const activeColumns = computed(() => {
  const ids = state.config?.ui.columns?.length ? state.config.ui.columns : Object.keys(COLUMNS)
  return ids.filter((id) => id in COLUMNS).map((id) => ({ id, ...COLUMNS[id] }))
})

export function selectFilter(kind: TreeFilter['kind'], id: string): void {
  state.filter = { kind, id }
}

export async function selectTask(id: string): Promise<void> {
  if (state.selectedId === id) return
  state.selectedId = id
  state.detail = null
  try {
    await Service.SelectTask(id)
  } catch (err) {
    state.lastError = String(err)
  }
}

export function setSort(key: string): void {
  if (state.sortKey === key) {
    state.sortDir = state.sortDir === 1 ? -1 : 1
    return
  }
  state.sortKey = key
  state.sortDir = 1
}

/** Persists the config, keeping the local copy in sync with what was stored. */
export async function persistConfig(): Promise<void> {
  if (!state.config) return
  state.busy = true
  try {
    state.config = toConfig(await Service.SaveConfig(state.config))
    state.warning = ''
  } catch (err) {
    state.lastError = `保存设置失败：${String(err)}`
  } finally {
    state.busy = false
  }
}

export async function setTheme(mode: AppConfig['ui']['theme']): Promise<void> {
  if (!state.config) return
  state.config.ui.theme = mode
  applyTheme(mode)
  await persistConfig()
}

/**
 * Splitter ratios are persisted separately from the rest of the config and
 * debounced, so dragging a divider does not rewrite the settings file dozens of
 * times per second.
 */
let layoutTimer: number | undefined
export function saveLayout(treeWidthRatio: number, detailHeightRatio: number): void {
  if (state.config) {
    state.config.ui.treeWidthRatio = treeWidthRatio
    state.config.ui.detailHeightRatio = detailHeightRatio
  }
  window.clearTimeout(layoutTimer)
  layoutTimer = window.setTimeout(() => {
    void Service.SaveLayout(treeWidthRatio, detailHeightRatio)
  }, 400)
}

export async function openFolder(dir: string): Promise<void> {
  try {
    await Service.OpenFolder(dir)
  } catch (err) {
    state.lastError = `打开目录失败：${String(err)}`
  }
}

/* ------------------------------------------------------------------ new task */

/** What the new-task dialog hands over. */
export interface NewTaskInput {
  url: string
  name: string
  dir: string
  category: string
  comment: string
  start: boolean
}

export function openTaskDialog(url = ''): void {
  state.dialogUrl = url
  state.dialogError = ''
  state.dialogOpen = true
}

export function closeTaskDialog(): void {
  if (state.dialogBusy) return
  state.dialogOpen = false
  state.dialogUrl = ''
  state.dialogError = ''
}

/**
 * Queues a download. Returns true when the dialog may close, so the component can
 * stay open on a failure and show the reason inline instead of dropping it.
 */
export async function addTask(input: NewTaskInput): Promise<boolean> {
  state.dialogBusy = true
  state.dialogError = ''
  try {
    await Service.AddTask({
      url: input.url,
      name: input.name,
      dir: input.dir,
      category: input.category,
      comment: input.comment,
      start: input.start,
    })
    state.dialogOpen = false
    state.dialogUrl = ''

    // Repaint at once rather than waiting up to 500ms for the next broadcast; the
    // event will overwrite this with the same content.
    state.snapshot = toSnapshot(await Service.SnapshotNow())
    // A task the user cannot see looks like a failure, so show where it landed.
    state.filter = { kind: 'status', id: 'downloading' }
    return true
  } catch (err) {
    state.dialogError = String(err)
    return false
  } finally {
    state.dialogBusy = false
  }
}

/** Asks the main window to open the dialog - this is how the floating window does it. */
export async function openTaskInMainWindow(url: string): Promise<void> {
  try {
    await Service.OpenNewTask(url)
  } catch (err) {
    state.lastError = `打开新建任务失败：${String(err)}`
  }
}

export async function pickFolder(startAt: string): Promise<string> {
  try {
    return await Service.PickFolder(startAt)
  } catch (err) {
    state.lastError = `打开目录选择器失败：${String(err)}`
    return ''
  }
}

/* --------------------------------------------------------------- task actions */

/**
 * Runs a task command and repaints from the engine immediately.
 *
 * The 500ms snapshot follows anyway, but a pause button that appears to do
 * nothing for half a second reads as broken.
 */
async function runTaskAction(what: string, run: () => Promise<unknown>): Promise<void> {
  try {
    await run()
    state.snapshot = toSnapshot(await Service.SnapshotNow())
  } catch (err) {
    state.lastError = `${what}失败：${String(err)}`
  }
}

export async function startTask(id: string): Promise<void> {
  if (!id) return
  await runTaskAction('开始任务', () => Service.StartTask(id))
}

export async function pauseTask(id: string): Promise<void> {
  if (!id) return
  await runTaskAction('暂停任务', () => Service.PauseTask(id))
}

export async function removeTask(id: string, deleteFile = false): Promise<void> {
  if (!id) return
  await runTaskAction(deleteFile ? '删除任务和文件' : '删除任务', () =>
    Service.RemoveTask(id, deleteFile),
  )
}

export async function startAllTasks(): Promise<void> {
  await runTaskAction('全部开始', () => Service.StartAllTasks())
}

export async function pauseAllTasks(): Promise<void> {
  await runTaskAction('全部暂停', () => Service.PauseAllTasks())
}

/* -------------------------------------------------------------------- drops */

let dropTimer: number | undefined

function announceDrop(text: string, ms = 3600): void {
  state.dropHint = text
  window.clearTimeout(dropTimer)
  dropTimer = window.setTimeout(() => {
    state.dropHint = ''
  }, ms)
}

/** A drop that was not a link: say what was expected instead of doing nothing. */
export function rejectDrop(): void {
  announceDrop('需要拖入链接（http / https）')
}

/**
 * Handles the native OS file drop that Go forwards as `files:dropped`.
 *
 * FlashDown speaks HTTP only, so a local path is not something it can queue.
 * Saying so beats swallowing the gesture.
 */
export function dropFiles(files: string[]): void {
  if (!files.length) return
  announceDrop('只支持 http / https 链接，本地文件无法下载')
}
