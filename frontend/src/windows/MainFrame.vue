<script setup lang="ts">
/**
 * The main window: FlashGet's six regions - menu bar, toolbar, category tree,
 * task grid, bottom detail pane, status bar - with the two splitters the spec
 * asks for.
 *
 * Splitter state is held as ratios, not pixels, so the layout survives a window
 * resize for free; the ratios are persisted (debounced) through saveLayout.
 */
import { onBeforeUnmount, onMounted, ref } from 'vue'
import MenuBar from '../components/MenuBar.vue'
import ToolBar from '../components/ToolBar.vue'
import CategoryTree from '../components/CategoryTree.vue'
import TaskGrid from '../components/TaskGrid.vue'
import DetailPane from '../components/DetailPane.vue'
import StatusBar from '../components/StatusBar.vue'
import TaskDialog from '../components/TaskDialog.vue'
import { useDropBasket } from '../lib/dropbasket'
import { openTaskDialog, saveLayout, state } from '../stores/session'

// The main window is a drop basket too: the spec requires a drop here to behave
// exactly as it does on the floating window - except the dialog is local here.
const { over, onDragOver, onDragLeave, onDrop } = useDropBasket(openTaskDialog)

/** Ranges mirror config.Normalize() so a drag can never write a value that gets rejected. */
const TREE_MIN = 0.1
const TREE_MAX = 0.45
const DETAIL_MIN = 0.12
const DETAIL_MAX = 0.7

const clamp = (v: number, lo: number, hi: number) => Math.min(hi, Math.max(lo, v))

// MainFrame only renders once the config has loaded, so this seed is safe.
const treeRatio = ref(clamp(state.config?.ui.treeWidthRatio ?? 0.2, TREE_MIN, TREE_MAX))
const detailRatio = ref(clamp(state.config?.ui.detailHeightRatio ?? 0.32, DETAIL_MIN, DETAIL_MAX))

const middle = ref<HTMLElement | null>(null)
const rightCol = ref<HTMLElement | null>(null)
const drag = ref<'tree' | 'detail' | ''>('')

function grab(which: 'tree' | 'detail', event: PointerEvent) {
  drag.value = which
  ;(event.currentTarget as HTMLElement).setPointerCapture(event.pointerId)
}

function moveTree(event: PointerEvent) {
  if (drag.value !== 'tree') return
  const rect = middle.value?.getBoundingClientRect()
  if (!rect?.width) return
  treeRatio.value = clamp((event.clientX - rect.left) / rect.width, TREE_MIN, TREE_MAX)
}

function moveDetail(event: PointerEvent) {
  if (drag.value !== 'detail') return
  const rect = rightCol.value?.getBoundingClientRect()
  if (!rect?.height) return
  detailRatio.value = clamp((event.clientY - rect.top) / rect.height, DETAIL_MIN, DETAIL_MAX)
}

function release() {
  if (!drag.value) return
  drag.value = ''
  saveLayout(treeRatio.value, detailRatio.value)
}

/** Arrow keys nudge the splitters, so the layout is reachable without a mouse. */
function nudge(which: 'tree' | 'detail', step: number) {
  if (which === 'tree') {
    treeRatio.value = clamp(treeRatio.value + step, TREE_MIN, TREE_MAX)
  } else {
    detailRatio.value = clamp(detailRatio.value + step, DETAIL_MIN, DETAIL_MAX)
  }
  saveLayout(treeRatio.value, detailRatio.value)
}

/**
 * Ctrl+N is advertised next to the menu item, so it has to work here rather than
 * only in the menu. Ignored while the dialog or a text field is already busy.
 */
function onKey(event: KeyboardEvent) {
  if (!(event.ctrlKey || event.metaKey) || event.key.toLowerCase() !== 'n') return
  event.preventDefault()
  if (!state.dialogOpen) openTaskDialog()
}

onMounted(() => window.addEventListener('keydown', onKey))
onBeforeUnmount(() => window.removeEventListener('keydown', onKey))
</script>

<template>
  <div
    class="shell"
    :class="{ dragging: !!drag, [`drag-${drag}`]: !!drag, over }"
    data-file-drop-target
    @dragover="onDragOver"
    @dragleave="onDragLeave"
    @drop="onDrop"
  >
    <MenuBar />
    <ToolBar />

    <div ref="middle" class="middle">
      <div class="tree-pane" :style="{ width: `${treeRatio * 100}%` }">
        <CategoryTree />
      </div>

      <div
        class="splitter v"
        role="separator"
        aria-orientation="vertical"
        tabindex="0"
        title="拖动调整类别树宽度"
        @pointerdown="grab('tree', $event)"
        @pointermove="moveTree"
        @pointerup="release"
        @pointercancel="release"
        @keydown.left.prevent="nudge('tree', -0.02)"
        @keydown.right.prevent="nudge('tree', 0.02)"
      />

      <div ref="rightCol" class="right">
        <TaskGrid />

        <div
          class="splitter h"
          role="separator"
          aria-orientation="horizontal"
          tabindex="0"
          title="拖动调整底部面板高度"
          @pointerdown="grab('detail', $event)"
          @pointermove="moveDetail"
          @pointerup="release"
          @pointercancel="release"
          @keydown.up.prevent="nudge('detail', -0.03)"
          @keydown.down.prevent="nudge('detail', 0.03)"
        />

        <div class="detail-pane" :style="{ height: `${detailRatio * 100}%` }">
          <DetailPane />
        </div>
      </div>
    </div>

    <StatusBar />

    <!-- Modal, so it is created on demand instead of living in the tree behind a
         display:none - the component owns its focus and key handling. -->
    <TaskDialog v-if="state.dialogOpen" />

    <!-- Drop feedback lives above the whole shell but never eats pointer events. -->
    <div v-if="over || state.dropHint" class="drop-layer" :class="{ armed: over }">
      <p class="drop-card">{{ over ? '松开以添加链接或文件' : state.dropHint }}</p>
    </div>
  </div>
</template>

<style scoped>
.shell {
  display: flex;
  flex-direction: column;
  height: 100vh;
  overflow: hidden;
  background: var(--app-bg);
  color: var(--text);
  font: var(--fs-body) var(--font-ui);
  user-select: none;
}

.middle {
  display: flex;
  flex: 1;
  min-height: 0;
}

.tree-pane {
  min-width: 120px;
  max-width: 46%;
  display: flex;
  min-height: 0;
}

.tree-pane > * {
  flex: 1;
  min-width: 0;
}

.right {
  display: flex;
  flex-direction: column;
  flex: 1;
  min-width: 0;
  min-height: 0;
}

.detail-pane {
  min-height: 60px;
  display: flex;
}

.detail-pane > * {
  flex: 1;
  min-width: 0;
}

.splitter {
  flex: 0 0 var(--splitter);
  background: var(--panel-alt);
  border: 0;
  position: relative;
}

.splitter::after {
  content: '';
  position: absolute;
  inset: 0;
  background: var(--border);
}

.splitter:hover::after,
.splitter:focus-visible::after,
.dragging .splitter::after {
  background: var(--accent);
}

.splitter.v {
  cursor: col-resize;
}

.splitter.h {
  cursor: row-resize;
}

.splitter:focus-visible {
  outline: none;
}

/* A single global cursor keeps the resize affordance while the pointer is
   captured - the splitter itself may be somewhere else entirely by then. */
.dragging.drag-tree {
  cursor: col-resize;
}

.dragging.drag-detail {
  cursor: row-resize;
}

/* Wails adds this class while an OS file drag hovers a drop target, so an
   Explorer drag gets the same affordance as a link drag (design.md D12). */
.shell.file-drop-target-active {
  outline: 2px dashed var(--accent);
  outline-offset: -2px;
}

/* Drop feedback sits above the shell but must never intercept the drag itself. */
.drop-layer {
  position: fixed;
  inset: 0;
  display: grid;
  place-items: end center;
  padding-bottom: calc(var(--statusbar-height) + 10px);
  pointer-events: none;
  z-index: 100;
}

.drop-card {
  margin: 0;
  max-width: 72%;
  padding: 7px 14px;
  border: 1px dashed var(--border-strong);
  border-radius: var(--radius);
  background: var(--panel-bg);
  box-shadow: var(--shadow-popup);
  color: var(--text-dim);
  font-size: var(--fs-body);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.drop-layer.armed .drop-card {
  border-style: solid;
  border-color: var(--accent);
  color: var(--text);
}
</style>
