<script setup lang="ts">
/**
 * Toolbar. The twelve buttons the spec calls for, in FlashGet's grouping.
 *
 * Buttons that need the download engine are disabled with an explanatory
 * tooltip instead of silently doing nothing - a visible, honest "not yet"
 * beats a dead button the user has to guess about.
 */
import { computed } from 'vue'
import {
  canPauseSelected,
  canStartSelected,
  openFolder,
  openTaskDialog,
  pauseTask,
  startTask,
  state,
} from '../stores/session'

interface Tool {
  id: string
  label: string
  /** Icons are stroke paths on a 16x16 grid; `filled` switches to a solid glyph. */
  path: string
  filled?: boolean
  disabled?: string
  action?: () => void
  /** Draws a group separator before this button. */
  groupStart?: boolean
}

const ENGINE = '下载引擎接入后可用（M1）'
const NO_START = '选中一个已暂停或出错的任务才能开始'
const NO_PAUSE = '选中一个正在下载的任务才能暂停'

// Start and pause only make sense for some states, so the toolbar asks the store
// rather than offering a button that quietly does nothing.
const tools = computed<Tool[]>(() => [
  {
    id: 'new',
    label: '新建',
    path: 'M4 2.5h5l3 3v8.5H4z M9 2.5v3h3 M8 8.2v4 M6 10.2h4',
    action: () => openTaskDialog(),
  },
  {
    id: 'batch',
    label: '添加成批任务',
    path: 'M5.5 2.5h6l2.5 2.5v7.5h-8.5z M3 5.2v8.5h8',
    disabled: ENGINE,
  },
  {
    id: 'start',
    label: '开始',
    path: 'M5 3.4l8.4 4.6L5 12.6z',
    filled: true,
    disabled: canStartSelected.value ? '' : NO_START,
    action: () => void startTask(state.selectedId),
  },
  {
    id: 'pause',
    label: '暂停',
    path: 'M5 3.4h2.3v9.2H5z M8.7 3.4H11v9.2H8.7z',
    filled: true,
    disabled: canPauseSelected.value ? '' : NO_PAUSE,
    action: () => void pauseTask(state.selectedId),
  },
  {
    id: 'delete',
    label: '删除',
    path: 'M3.4 5h9.2 M6.4 5V3.4h3.2V5 M4.8 5l.7 8.6h5l.7-8.6 M6.9 7.4v4.2 M9.1 7.4v4.2',
    disabled: ENGINE,
  },
  {
    id: 'props',
    label: '属性',
    path: 'M4 2.5h5.5L12 5v8.5H4z M9.5 2.5V5H12 M6.5 8h4 M6.5 10.5h4',
    disabled: ENGINE,
  },
  {
    id: 'folder',
    label: '目录',
    path: 'M2.2 4.4h3.9l1.3 1.9h6.4v7.1H2.2z',
    groupStart: true,
    action: () => void openFolder(state.config?.downloadRoot ?? ''),
  },
  { id: 'find', label: '查找', path: 'M7 3a4 4 0 100 8 4 4 0 000-8z M10.1 10.1l3.4 3.4', disabled: '查找（M4）' },
  { id: 'up', label: '上移', path: 'M8 12.6V4.4 M4.6 7.8L8 4.4l3.4 3.4', groupStart: true, disabled: ENGINE },
  { id: 'down', label: '下移', path: 'M8 3.4v8.2 M4.6 8.2L8 11.6l3.4-3.4', disabled: ENGINE },
  {
    id: 'options',
    label: '选项',
    path: 'M8 6.1a1.9 1.9 0 100 3.8 1.9 1.9 0 000-3.8z M8 2.2v1.7 M8 12.1v1.7 M2.2 8h1.7 M12.1 8h1.7 M3.9 3.9l1.2 1.2 M10.9 10.9l1.2 1.2 M12.1 3.9l-1.2 1.2 M5.1 10.9l-1.2 1.2',
    groupStart: true,
    disabled: '选项对话框（M3–M4）',
  },
])
</script>

<template>
  <div class="toolbar">
    <template v-for="tool in tools" :key="tool.id">
      <i v-if="tool.groupStart" class="sep" />
      <button
        type="button"
        class="tool"
        :class="{ disabled: !!tool.disabled }"
        :disabled="!!tool.disabled"
        :title="tool.disabled || tool.label"
        :aria-label="tool.label"
        @click="tool.action?.()"
      >
        <svg viewBox="0 0 16 16" aria-hidden="true">
          <path
            :d="tool.path"
            :fill="tool.filled ? 'currentColor' : 'none'"
            :stroke="tool.filled ? 'none' : 'currentColor'"
          />
        </svg>
      </button>
    </template>
  </div>
</template>

<style scoped>
.toolbar {
  display: flex;
  align-items: center;
  gap: 1px;
  height: var(--toolbar-height);
  flex: 0 0 auto;
  padding: 0 4px;
  background: linear-gradient(var(--toolbar-top), var(--toolbar-bottom));
  border-bottom: 1px solid var(--border);
}

.tool {
  display: grid;
  place-items: center;
  width: 24px;
  height: 24px;
  padding: 0;
  border: 1px solid transparent;
  border-radius: var(--radius);
  background: transparent;
  color: var(--text);
  cursor: default;
}

.tool svg {
  width: 16px;
  height: 16px;
  stroke-width: 1.15;
  stroke-linecap: round;
  stroke-linejoin: round;
}

.tool:not(.disabled):hover {
  background: var(--accent-soft);
  border-color: var(--row-selected-border);
}

.tool:not(.disabled):active {
  background: var(--row-selected);
}

.tool.disabled {
  color: var(--text-faint);
  opacity: 0.65;
}

.sep {
  width: 1px;
  height: 18px;
  margin: 0 4px;
  background: var(--border);
}
</style>
