<script setup lang="ts">
/**
 * The floating window: 220x92, frameless, always on top.
 *
 * It renders from the same session store as the main window, which works
 * because `speed:tick` is broadcast to every window - the graph here costs no
 * extra IPC (design.md D7/D11).
 */
import Sparkline from '../components/Sparkline.vue'
import { useDropBasket } from '../lib/dropbasket'
import { formatSpeed } from '../lib/format'
import { openTaskInMainWindow, state } from '../stores/session'

// A drop here has no dialog of its own: ask the main window to open one.
const { over, onDragOver, onDragLeave, onDrop } = useDropBasket(openTaskInMainWindow)
</script>

<template>
  <div
    class="float"
    :class="{ over, hinted: !!state.dropHint }"
    data-file-drop-target
    @dragover="onDragOver"
    @dragleave="onDragLeave"
    @drop="onDrop"
  >
    <div class="head">
      <span class="value">{{ formatSpeed(state.speed) }}</span>
      <span class="busy">{{ state.snapshot.downloading }} 下载中</span>
    </div>

    <Sparkline :values="state.speedHistory" :width="196" :height="26" class="graph" />

    <div class="foot">
      <!-- A captured link takes over the footer until it expires; the rest of the
           time it just reports the active connection count. -->
      <span class="tip">{{ state.dropHint || `Jet ${state.snapshot.activeJets}` }}</span>
      <span v-if="state.lastError" class="alert" :title="state.lastError">!</span>
      <span v-else-if="state.warning" class="warn" :title="state.warning">!</span>
    </div>
  </div>
</template>

<style scoped>
/*
 * Frameless-window contracts (design.md D11) - the values must be exactly these
 * keywords; a near-miss silently does nothing:
 *   --wails-draggable: drag            the whole body drags the window
 *   --custom-contextmenu: float-menu   the native menu registered by
 *                                      app/menu.go; the two ids must agree
 *   --default-contextmenu: hide        suppresses the WebView's own menu
 */
.float {
  --wails-draggable: drag;
  --custom-contextmenu: float-menu;
  --default-contextmenu: hide;

  box-sizing: border-box;
  display: flex;
  flex-direction: column;
  gap: 2px;
  height: 100vh;
  padding: 4px 8px 3px;
  overflow: hidden;
  background: var(--panel-bg);
  border: 1px solid var(--border-strong);
  color: var(--text);
  font: var(--fs-body) var(--font-ui);
  user-select: none;
}

/* `over` covers DOM (link) drags; the class Wails adds covers OS file drags,
   which never reach the DOM at all (design.md D12). */
.float.over,
.float.file-drop-target-active {
  border-color: var(--accent);
  background: var(--accent-soft);
}

.head {
  display: flex;
  align-items: baseline;
  justify-content: space-between;
  gap: 6px;
}

.value {
  font-family: var(--font-mono);
  font-size: 17px;
  font-weight: 600;
  font-variant-numeric: tabular-nums;
  letter-spacing: -0.2px;
  color: var(--text);
}

.busy {
  color: var(--text-dim);
  font-size: var(--fs-small);
}

.graph {
  width: 100%;
  height: 26px;
}

.foot {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 6px;
  color: var(--text-faint);
  font-size: var(--fs-small);
  line-height: 1.2;
}

.tip {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

/* The hint is a sentence, not a number, so it gets two lines when it needs one. */
.hinted .tip {
  color: var(--accent);
  white-space: normal;
}

.alert {
  flex: 0 0 auto;
  padding: 0 4px;
  border-radius: 2px;
  background: var(--danger);
  color: var(--text-invert);
}

.warn {
  flex: 0 0 auto;
  padding: 0 4px;
  border-radius: 2px;
  background: var(--warn);
  color: var(--text-invert);
}
</style>
