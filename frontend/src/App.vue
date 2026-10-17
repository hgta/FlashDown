<script setup lang="ts">
import { computed, onMounted } from 'vue'
import MainFrame from './windows/MainFrame.vue'
import FloatWindow from './windows/FloatWindow.vue'
import { init, state } from './stores/session'

// Both windows load the same bundle; the query string decides which one renders.
const isFloat = computed(
  () => new URLSearchParams(window.location.search).get('win') === 'float',
)

onMounted(() => {
  void init()
})
</script>

<template>
  <FloatWindow v-if="isFloat" />
  <MainFrame v-else-if="state.ready" />
  <div v-else class="boot">FlashDown 正在启动…</div>
</template>

<style scoped>
.boot {
  position: fixed;
  inset: 0;
  display: grid;
  place-items: center;
  background: var(--app-bg);
  color: var(--text-dim);
  font: var(--fs-body) var(--font-ui);
}
</style>
