<script setup lang="ts">
/**
 * The task grid: FlashGet's column order, an in-cell progress bar, and row
 * virtualisation so a 1000-task list stays smooth while downloads are running.
 *
 * Virtualisation is deliberately hand-rolled rather than pulled from a library:
 * rows are a fixed height, so the visible slice is pure arithmetic and the
 * render path stays at a handful of DOM nodes.
 */
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import {
  COLUMNS,
  activeColumns,
  openFolder,
  persistConfig,
  selectTask,
  setSort,
  state,
  visibleTasks,
} from '../stores/session'
import {
  formatBytes,
  formatDuration,
  formatPercent,
  formatRemaining,
  formatSpeed,
  percent,
} from '../lib/format'
import type { TaskDTO } from '../types'

const ROW_H = 24
/** Rows rendered beyond the viewport, so fast scrolling never shows a gap. */
const OVERSCAN = 6

const scroller = ref<HTMLElement | null>(null)
const scrollTop = ref(0)
const viewportH = ref(0)

const rows = visibleTasks
const totalHeight = computed(() => rows.value.length * ROW_H)
const firstIndex = computed(() => Math.max(0, Math.floor(scrollTop.value / ROW_H) - OVERSCAN))
const lastIndex = computed(() =>
  Math.min(rows.value.length, Math.ceil((scrollTop.value + viewportH.value) / ROW_H) + OVERSCAN),
)
const slice = computed(() =>
  rows.value.slice(firstIndex.value, lastIndex.value).map((task, i) => {
    const index = firstIndex.value + i
    // Striping follows the absolute index, not the DOM position, so stripes do
    // not flicker as the virtual window slides during a scroll.
    return { task, top: index * ROW_H, alt: index % 2 === 1 }
  }),
)

function onScroll() {
  scrollTop.value = scroller.value?.scrollTop ?? 0
}

function measure() {
  viewportH.value = scroller.value?.clientHeight ?? 0
}

let observer: ResizeObserver | undefined
onMounted(() => {
  measure()
  observer = new ResizeObserver(measure)
  if (scroller.value) observer.observe(scroller.value)
  document.addEventListener('mousedown', onDocumentDown)
})
onBeforeUnmount(() => {
  observer?.disconnect()
  document.removeEventListener('mousedown', onDocumentDown)
})

/** Sorting: clicking the active column flips direction, a new column starts ascending. */
function headerClick(id: string) {
  setSort(id)
}

function sortMark(id: string): string {
  if (state.sortKey !== id) return ''
  return state.sortDir === 1 ? '▲' : '▼'
}

/* ---------------------------------------------------------------- selection */

async function rowClick(task: TaskDTO) {
  await selectTask(task.id)
}

/** Keyboard navigation keeps the grid usable without a mouse, as a grid should. */
function onKey(event: KeyboardEvent) {
  if (!rows.value.length) return
  const current = rows.value.findIndex((t) => t.id === state.selectedId)

  let next = current
  if (event.key === 'ArrowDown') next = Math.min(rows.value.length - 1, current + 1)
  else if (event.key === 'ArrowUp') next = Math.max(0, current - 1)
  else if (event.key === 'Home') next = 0
  else if (event.key === 'End') next = rows.value.length - 1
  else return

  event.preventDefault()
  if (current < 0 && (event.key === 'ArrowUp' || event.key === 'ArrowDown')) next = 0
  void selectTask(rows.value[next].id)
  revealRow(next)
}

function revealRow(index: number) {
  const el = scroller.value
  if (!el) return
  const top = index * ROW_H
  if (top < el.scrollTop) el.scrollTop = top
  else if (top + ROW_H > el.scrollTop + el.clientHeight) el.scrollTop = top + ROW_H - el.clientHeight
}

/** Double-clicking a finished task reveals it in the file manager. */
function rowDblClick(task: TaskDTO) {
  if (task.state !== 'finished') return
  const dir =
    state.config?.categories.find((c) => c.id === task.category)?.dir ??
    state.config?.downloadRoot ??
    ''
  void openFolder(dir)
}

/* -------------------------------------------------------------- cell render */

/**
 * Only states a progress bar cannot express get a badge. A task that cannot be
 * resumed keeps downloading normally, but the user deserves to know that a
 * crash will cost the whole file.
 */
const STATE_BADGES: Record<string, { text: string; kind: string }> = {
  draft: { text: '草稿', kind: 'idle' },
  waiting: { text: '等待', kind: 'idle' },
  probing: { text: '探测', kind: 'idle' },
  paused: { text: '已暂停', kind: 'idle' },
  verifying: { text: '校验中', kind: 'busy' },
  completing: { text: '收尾中', kind: 'busy' },
  removed: { text: '已删除', kind: 'idle' },
  error: { text: '出错', kind: 'error' },
}

function badgeOf(task: TaskDTO): { text: string; kind: string } | null {
  if (!task.resumable && task.state !== 'finished') {
    return task.state === 'error' ? STATE_BADGES.error : { text: '不可续传', kind: 'warn' }
  }
  return STATE_BADGES[task.state] ?? null
}

function rowTitle(task: TaskDTO): string {
  return [task.name, task.url, task.error, task.comment].filter(Boolean).join('\n')
}

function progressClass(task: TaskDTO): string {
  if (task.state === 'error') return 'error'
  if (task.state === 'paused' || task.state === 'removed' || task.state === 'draft') return 'paused'
  if (task.state === 'finished') return 'done'
  return ''
}

/* --------------------------------------------------------- column visibility */

const menu = ref<{ x: number; y: number } | null>(null)
const headerEl = ref<HTMLElement | null>(null)

function openMenu(event: MouseEvent) {
  const rect = headerEl.value?.getBoundingClientRect()
  menu.value = {
    x: Math.max(0, event.clientX - (rect?.left ?? 0)),
    y: (rect?.height ?? 0) + 2,
  }
}

function onDocumentDown() {
  menu.value = null
}

function isVisible(id: string): boolean {
  return (state.config?.ui.columns ?? Object.keys(COLUMNS)).includes(id)
}

/** Hiding a column drops it from the persisted order; re-showing appends it. */
async function toggleColumn(id: string) {
  const cfg = state.config
  if (!cfg) return
  if (!cfg.ui.columns?.length) cfg.ui.columns = Object.keys(COLUMNS)
  if (cfg.ui.columns.includes(id)) {
    // Never hide the last column - an empty grid is a dead end.
    if (cfg.ui.columns.length <= 1) return
    cfg.ui.columns = cfg.ui.columns.filter((c) => c !== id)
  } else {
    cfg.ui.columns = [...cfg.ui.columns, id]
  }
  await persistConfig()
}
</script>

<template>
  <div class="grid">
    <div ref="headerEl" class="header" @contextmenu.prevent="openMenu">
      <div
        v-for="col in activeColumns"
        :key="col.id"
        class="cell"
        :class="[`c-${col.id}`, { right: col.align === 'right' }]"
        :style="col.width ? { width: `${col.width}px`, flex: '0 0 auto' } : {}"
        @click="headerClick(col.id)"
      >
        <span>{{ col.title }}</span>
        <span class="mark">{{ sortMark(col.id) }}</span>
      </div>
    </div>

    <!-- A right-click on the header edits which columns are shown. -->
    <div
      v-if="menu"
      class="colmenu"
      :style="{ left: `${menu.x}px`, top: `${menu.y}px` }"
      @mousedown.stop
    >
      <div class="colmenu-title">显示列</div>
      <button
        v-for="(col, id) in COLUMNS"
        :key="id"
        type="button"
        class="colmenu-item"
        @click="toggleColumn(id)"
      >
        <span class="check">{{ isVisible(id) ? '✓' : '' }}</span>
        <span>{{ col.title }}</span>
      </button>
      <div class="colmenu-hint">列顺序配置随 M3 提供</div>
    </div>

    <div ref="scroller" class="scroller" tabindex="0" @scroll="onScroll" @keydown="onKey">
      <div class="spacer" :style="{ height: `${totalHeight}px` }">
        <div
          v-for="entry in slice"
          :key="entry.task.id"
          class="row"
          :class="{ selected: entry.task.id === state.selectedId, alt: entry.alt }"
          :style="{ top: `${entry.top}px` }"
          :title="rowTitle(entry.task)"
          @click="rowClick(entry.task)"
          @dblclick="rowDblClick(entry.task)"
        >
          <div
            v-for="col in activeColumns"
            :key="col.id"
            class="cell"
            :class="[`c-${col.id}`, { right: col.align === 'right' }]"
            :style="col.width ? { width: `${col.width}px`, flex: '0 0 auto' } : {}"
          >
            <template v-if="col.id === 'name'">
              <span
                v-if="badgeOf(entry.task)"
                class="badge"
                :class="badgeOf(entry.task)!.kind"
              >{{ badgeOf(entry.task)!.text }}</span>
              <span class="name">{{ entry.task.name }}</span>
            </template>

            <template v-else-if="col.id === 'size'">
              {{ formatBytes(entry.task.size) }}
            </template>

            <template v-else-if="col.id === 'progress'">
              <span class="bar">
                <span
                  class="fill"
                  :class="progressClass(entry.task)"
                  :style="{ width: `${percent(entry.task.done, entry.task.size)}%` }"
                />
              </span>
              <span class="pct">{{ formatPercent(entry.task.done, entry.task.size) }}</span>
            </template>

            <template v-else-if="col.id === 'elapsed'">
              {{ formatDuration(entry.task.elapsedMs) }}
            </template>

            <template v-else-if="col.id === 'remaining'">
              {{ formatRemaining(entry.task.remainMs, entry.task.speed) }}
            </template>

            <template v-else-if="col.id === 'speed'">
              {{ formatSpeed(entry.task.speed) }}
            </template>

            <template v-else-if="col.id === 'blocks'">
              {{ entry.task.blocks || '—' }}
            </template>
          </div>
        </div>
      </div>

      <p v-if="!rows.length" class="empty">此分类下暂无任务</p>
    </div>
  </div>
</template>

<style scoped>
.grid {
  display: flex;
  flex-direction: column;
  min-height: 0;
  flex: 1;
  background: var(--panel-bg);
  position: relative;
}

.header {
  display: flex;
  align-items: center;
  height: var(--header-height);
  flex: 0 0 auto;
  background: linear-gradient(var(--header-top), var(--header-bottom));
  border-bottom: 1px solid var(--border-strong);
  color: var(--text-dim);
  font-size: var(--fs-small);
  user-select: none;
}

.header .cell {
  gap: 3px;
}

.header .cell:hover {
  background: var(--row-hover);
  color: var(--text);
}

.mark {
  color: var(--accent);
  font-size: 8px;
}

.cell {
  display: flex;
  align-items: center;
  min-width: 0;
  padding: 0 6px;
  overflow: hidden;
}

.cell.right {
  justify-content: flex-end;
}

.c-name {
  flex: 1 1 auto;
  gap: 5px;
}

.scroller {
  flex: 1;
  min-height: 0;
  overflow-y: auto;
  outline: none;
  position: relative;
}

.scroller:focus-visible {
  box-shadow: inset var(--focus);
}

.spacer {
  position: relative;
}

.row {
  position: absolute;
  left: 0;
  right: 0;
  display: flex;
  align-items: center;
  height: 24px;
  cursor: default;
  color: var(--text);
  font-variant-numeric: tabular-nums;
}

.row.alt {
  background: var(--row-alt);
}

.row:hover {
  background: var(--row-hover);
}

.row.selected {
  background: var(--row-selected);
  box-shadow: inset 0 0 0 1px var(--row-selected-border);
}

.name {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.badge {
  flex: 0 0 auto;
  padding: 0 4px;
  border-radius: 2px;
  background: var(--panel-sunken);
  border: 1px solid var(--border);
  color: var(--text-dim);
  font-size: 10px;
  line-height: 14px;
}

.badge.warn {
  color: var(--warn);
  border-color: var(--warn);
}

.badge.error {
  color: var(--danger);
  border-color: var(--danger);
}

.badge.busy {
  color: var(--accent);
  border-color: var(--accent);
}

.bar {
  flex: 1 1 auto;
  height: 10px;
  border-radius: 2px;
  background: var(--progress-track);
  overflow: hidden;
}

.fill {
  display: block;
  height: 100%;
  background: var(--progress-fill);
}

.fill.paused {
  background: var(--progress-fill-paused);
}

.fill.error {
  background: var(--progress-fill-error);
}

.pct {
  flex: 0 0 42px;
  margin-left: 5px;
  text-align: right;
  color: var(--text-dim);
  font-size: var(--fs-small);
}

.empty {
  position: absolute;
  inset: 0;
  display: grid;
  place-items: center;
  margin: 0;
  color: var(--text-faint);
}

.colmenu {
  position: absolute;
  z-index: 60;
  min-width: 132px;
  padding: 4px 0;
  background: var(--panel-bg);
  border: 1px solid var(--border-strong);
  border-radius: var(--radius);
  box-shadow: var(--shadow-popup);
  font-size: var(--fs-menu);
}

.colmenu-title {
  padding: 2px 10px 4px;
  color: var(--text-faint);
  font-size: var(--fs-small);
}

.colmenu-item {
  display: flex;
  align-items: center;
  gap: 6px;
  width: 100%;
  padding: 3px 10px 3px 6px;
  border: 0;
  background: transparent;
  color: var(--text);
  font: inherit;
  text-align: left;
  cursor: default;
}

.colmenu-item:hover {
  background: var(--accent);
  color: var(--text-invert);
}

.colmenu-item .check {
  width: 12px;
  flex: 0 0 12px;
  text-align: center;
}

.colmenu-hint {
  margin-top: 3px;
  padding: 3px 10px 0;
  border-top: 1px solid var(--border);
  color: var(--text-faint);
  font-size: var(--fs-small);
}
</style>
