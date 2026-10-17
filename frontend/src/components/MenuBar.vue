<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import {
  canPauseSelected,
  canStartSelected,
  JET_CHOICES,
  maxJets,
  openFolder,
  openTaskDialog,
  pauseTask,
  persistConfig,
  setMaxJets,
  setTheme,
  startTask,
  state,
} from '../stores/session'

interface Item {
  label?: string
  accel?: string
  separator?: boolean
  disabled?: boolean
  checked?: boolean
  action?: () => void
}

interface Menu {
  label: string
  items: Item[]
}

/** Items needing the engine are disabled until M1 lands, rather than silently doing nothing. */
const PENDING = '下载引擎接入后可用（M1）'

const openIndex = ref<number | null>(null)

function toggle(i: number) {
  openIndex.value = openIndex.value === i ? null : i
}

function hover(i: number) {
  if (openIndex.value !== null) openIndex.value = i
}

function run(item: Item) {
  if (item.disabled || item.separator) return
  openIndex.value = null
  item.action?.()
}

function close() {
  openIndex.value = null
}

function onKey(event: KeyboardEvent) {
  if (event.key === 'Escape') close()
}

onMounted(() => document.addEventListener('keydown', onKey))
onBeforeUnmount(() => document.removeEventListener('keydown', onKey))

const menus = computed<Menu[]>(() => [
  {
    label: '文件',
    items: [
      { label: '新建任务…', accel: 'Ctrl+N', action: () => openTaskDialog() },
      { label: '添加成批任务…', disabled: true },
      { separator: true },
      { label: '导入未完成任务…', disabled: true },
      { label: '导出 URL 列表…', disabled: true },
      { separator: true },
      { label: '退出', accel: 'Alt+F4', action: () => window.close() },
    ],
  },
  {
    label: '编辑',
    items: [
      { label: '全选', accel: 'Ctrl+A', disabled: true },
      { label: '反向选择', disabled: true },
      { separator: true },
      { label: '删除任务', disabled: true },
      { label: '删除任务和文件', disabled: true },
    ],
  },
  {
    label: '查看',
    items: [
      { label: '正在下载', checked: state.filter.id === 'downloading', action: () => (state.filter = { kind: 'status', id: 'downloading' }) },
      { label: '已完成', checked: state.filter.id === 'finished', action: () => (state.filter = { kind: 'status', id: 'finished' }) },
      { label: '已删除', checked: state.filter.id === 'removed', action: () => (state.filter = { kind: 'status', id: 'removed' }) },
      { separator: true },
      { label: '主题：跟随系统', checked: state.config?.ui.theme === 'system', action: () => void setTheme('system') },
      { label: '主题：浅色', checked: state.config?.ui.theme === 'light', action: () => void setTheme('light') },
      { label: '主题：深色', checked: state.config?.ui.theme === 'dark', action: () => void setTheme('dark') },
    ],
  },
  {
    label: '任务',
    items: [
      {
        label: '开始',
        disabled: !canStartSelected.value,
        action: () => void startTask(state.selectedId),
      },
      {
        label: '暂停',
        disabled: !canPauseSelected.value,
        action: () => void pauseTask(state.selectedId),
      },
      { separator: true },
      { label: '全部开始', disabled: true },
      { label: '全部暂停', disabled: true },
      { separator: true },
      { label: '上移', disabled: true },
      { label: '下移', disabled: true },
      { separator: true },
      // FlashGet's connection count: raised or lowered while downloading, the
      // engine reconciles the jets on its next tick.
      ...JET_CHOICES.map((n) => ({
        label: `连接数：${n}`,
        checked: maxJets.value === n,
        action: () => void setMaxJets(n),
      })),
      { separator: true },
      { label: '属性…', disabled: true },
    ],
  },
  {
    label: '工具',
    items: [
      { label: '选项…', disabled: true },
      { separator: true },
      {
        label: '打开下载目录',
        action: () => void openFolder(state.config?.downloadRoot ?? ''),
      },
      { label: '打开配置目录', action: () => void openFolder(dirOf(state.configPath)) },
      {
        label: '保存设置',
        disabled: state.busy,
        action: () => void persistConfig(),
      },
    ],
  },
  {
    label: '帮助',
    items: [{ label: `关于 FlashDown ${state.version}`, action: () => (openIndex.value = null) }],
  },
])

function dirOf(filePath: string): string {
  if (!filePath) return ''
  return filePath.slice(0, Math.max(filePath.lastIndexOf('\\'), filePath.lastIndexOf('/')))
}
</script>

<template>
  <!-- Clicking anywhere else closes the open menu. -->
  <div class="menubar" @mouseleave="close">
    <div
      v-for="(menu, i) in menus"
      :key="menu.label"
      class="menu"
      :class="{ open: openIndex === i }"
    >
      <button class="title" type="button" @click="toggle(i)" @mouseenter="hover(i)">
        {{ menu.label }}
      </button>

      <div v-if="openIndex === i" class="dropdown">
        <template v-for="(item, j) in menu.items" :key="j">
          <div v-if="item.separator" class="sep" />
          <button
            v-else
            class="item"
            type="button"
            :class="{ disabled: item.disabled }"
            :title="item.disabled ? PENDING : ''"
            @click="run(item)"
          >
            <span class="check">{{ item.checked ? '✓' : '' }}</span>
            <span class="label">{{ item.label }}</span>
            <span v-if="item.accel" class="accel">{{ item.accel }}</span>
          </button>
        </template>
      </div>
    </div>
  </div>
</template>

<style scoped>
.menubar {
  display: flex;
  align-items: stretch;
  height: var(--menubar-height);
  flex: 0 0 auto;
  background: var(--panel-bg);
  border-bottom: 1px solid var(--border);
  font-size: var(--fs-menu);
  position: relative;
  z-index: 40;
}

.menu {
  position: relative;
}

.title {
  height: 100%;
  padding: 0 10px;
  border: 0;
  background: transparent;
  color: var(--text);
  font: inherit;
  cursor: default;
}

.menu.open > .title,
.title:hover {
  background: var(--accent-soft);
}

.dropdown {
  position: absolute;
  top: 100%;
  left: 0;
  min-width: 208px;
  padding: 4px 0;
  background: var(--panel-bg);
  border: 1px solid var(--border-strong);
  border-radius: var(--radius);
  box-shadow: var(--shadow-popup);
  z-index: 50;
  /* The task menu grew past a short window: scroll instead of clipping items
     the user cannot reach. */
  max-height: calc(100vh - var(--menubar-height) - 8px);
  overflow-y: auto;
}

.item {
  display: flex;
  align-items: center;
  gap: 6px;
  width: 100%;
  padding: 4px 10px 4px 6px;
  border: 0;
  background: transparent;
  color: var(--text);
  font: inherit;
  text-align: left;
  cursor: default;
}

.item:not(.disabled):hover {
  background: var(--accent);
  color: var(--text-invert);
}

.item.disabled {
  color: var(--text-faint);
}

.check {
  width: 12px;
  flex: 0 0 12px;
  text-align: center;
}

.label {
  flex: 1;
  white-space: nowrap;
}

.accel {
  color: var(--text-faint);
  font-size: var(--fs-small);
}

.item:not(.disabled):hover .accel {
  color: var(--text-invert);
  opacity: 0.75;
}

.sep {
  height: 1px;
  margin: 4px 8px;
  background: var(--border);
}
</style>
