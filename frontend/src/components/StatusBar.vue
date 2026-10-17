<script setup lang="ts">
import Sparkline from './Sparkline.vue'
import { formatSpeed } from '../lib/format'
import { state } from '../stores/session'
</script>

<template>
  <div class="status">
    <span>共 {{ state.snapshot.tasks.length }} 个任务</span>
    <i class="sep" />
    <span>{{ state.snapshot.downloading }} 个正在下载</span>
    <i class="sep" />
    <span>Jet {{ state.snapshot.activeJets }}</span>

    <span class="grow" />

    <span v-if="state.lastError" class="err" :title="state.lastError">{{ state.lastError }}</span>
    <span v-else-if="state.warning" class="warn" :title="state.warning">{{ state.warning }}</span>

    <span class="grow" />

    <span class="mono speed">{{ formatSpeed(state.speed) }}</span>
    <Sparkline :values="state.speedHistory" :width="76" :height="14" />
  </div>
</template>

<style scoped>
.status {
  display: flex;
  align-items: center;
  gap: 8px;
  height: var(--statusbar-height);
  flex: 0 0 auto;
  padding: 0 8px;
  background: var(--panel-bg);
  border-top: 1px solid var(--border);
  color: var(--text-dim);
  font-size: var(--fs-small);
}

.grow {
  flex: 1;
}

.sep {
  width: 1px;
  height: 12px;
  background: var(--border);
}

.mono {
  font-family: var(--font-mono);
  font-variant-numeric: tabular-nums;
}

.speed {
  color: var(--text);
  min-width: 78px;
  text-align: right;
}

.warn {
  color: var(--warn);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  max-width: 45%;
}

.err {
  color: var(--danger);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  max-width: 45%;
}
</style>
