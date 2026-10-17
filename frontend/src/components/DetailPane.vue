<script setup lang="ts">
/**
 * The bottom pane: block map, jet list and log for the selected task.
 *
 * The block map is a <canvas>, not ~300 DOM nodes - the detail event arrives
 * every 250ms and rebuilding a few hundred elements at that rate is exactly the
 * kind of thing that makes a WebView stutter.
 */
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { state } from '../stores/session'
import { formatBytes, formatSpeed, jetColor, percent } from '../lib/format'

type Tab = 'map' | 'jet' | 'log'
const tab = ref<Tab>('map')

const detail = computed(() => state.detail)
const jets = computed(() => detail.value?.jets ?? [])
const log = computed(() => detail.value?.log ?? [])

const CELL_W = 7
const CELL_H = 11
/** 4 fps - the spec's ceiling for the map, and plenty for a progress view. */
const FRAME_MS = 250

const wrap = ref<HTMLElement | null>(null)
const canvas = ref<HTMLCanvasElement | null>(null)
const logBox = ref<HTMLElement | null>(null)

const pending = computed(() => (detail.value?.blocks ?? []).filter((b) => b.done <= 0).length)
const running = computed(() => (detail.value?.blocks ?? []).filter((b) => b.jet >= 0 && b.done < b.end - b.start).length)

function draw() {
  const el = canvas.value
  const box = wrap.value
  if (!el || !box) return

  const dpr = window.devicePixelRatio || 1
  const width = Math.max(1, box.clientWidth)
  const blocks = detail.value?.blocks ?? []

  const cols = Math.max(1, Math.floor(width / (CELL_W + 1)))
  const rows = Math.max(1, Math.ceil(blocks.length / cols))
  const height = rows * (CELL_H + 1)

  el.width = Math.round(width * dpr)
  el.height = Math.round(height * dpr)
  el.style.width = `${width}px`
  el.style.height = `${height}px`

  const ctx = el.getContext('2d')
  if (!ctx) return
  ctx.setTransform(dpr, 0, 0, dpr, 0, 0)
  ctx.clearRect(0, 0, width, height)

  const css = getComputedStyle(document.documentElement)
  const track = css.getPropertyValue('--progress-track').trim() || '#ddd'
  const fill = css.getPropertyValue('--progress-fill').trim() || '#3aa757'

  blocks.forEach((block, i) => {
    const x = (i % cols) * (CELL_W + 1)
    const y = Math.floor(i / cols) * (CELL_H + 1)

    const span = Math.max(1, block.end - block.start)
    const complete = block.done >= span

    if (complete) {
      ctx.fillStyle = fill
    } else if (block.done > 0 || block.jet >= 0) {
      // Claimed or partially written: colour by the jet so parallel activity is
      // readable at a glance. A partially filled cell gets a faint column.
      ctx.fillStyle = track
      ctx.fillRect(x, y, CELL_W, CELL_H)
      ctx.fillStyle = jetColor(block.jet, block.done > 0 ? 1 : 0.38)
      ctx.fillRect(x, y, CELL_W, Math.max(1, Math.round((CELL_H * block.done) / span)))
      return
    } else {
      ctx.fillStyle = track
    }
    ctx.fillRect(x, y, CELL_W, CELL_H)
  })
}

let raf = 0
let lastDraw = 0

function loop(ts: number) {
  raf = requestAnimationFrame(loop)
  if (ts - lastDraw < FRAME_MS) return
  lastDraw = ts
  draw()
}

function start() {
  if (!raf) raf = requestAnimationFrame(loop)
}

function stop() {
  if (raf) cancelAnimationFrame(raf)
  raf = 0
}

watch(
  [tab, () => state.selectedId],
  () => {
    if (tab.value === 'map' && state.selectedId) start()
    else stop()
    if (tab.value === 'map') draw()
  },
  { immediate: true },
)

// A fresh detail payload should appear at once rather than on the next frame
// tick, otherwise the map looks a quarter-second stale after every click.
watch(detail, () => {
  if (tab.value === 'map') draw()
})

watch(log, () => {
  const box = logBox.value
  if (box) box.scrollTop = box.scrollHeight
})

let observer: ResizeObserver | undefined
onMounted(() => {
  observer = new ResizeObserver(() => {
    if (tab.value === 'map') draw()
  })
  if (wrap.value) observer.observe(wrap.value)
  draw()
})

onBeforeUnmount(() => {
  stop()
  observer?.disconnect()
})
</script>

<template>
  <div class="detail">
    <div class="tabs">
      <button type="button" :class="{ on: tab === 'map' }" @click="tab = 'map'">块图</button>
      <button type="button" :class="{ on: tab === 'jet' }" @click="tab = 'jet'">
        Jet<span class="n">{{ jets.length }}</span>
      </button>
      <button type="button" :class="{ on: tab === 'log' }" @click="tab = 'log'">
        日志<span class="n">{{ log.length }}</span>
      </button>
      <span class="grow" />
      <span v-if="tab === 'map' && detail" class="legend">
        <i class="swatch done" />已完成 {{ (detail.blocks.length || 0) - pending - running }}
        <i class="swatch active" />下载中 {{ running }}
        <i class="swatch idle" />未开始 {{ pending }}
      </span>
    </div>

    <div class="body">
      <p v-if="!state.selectedId" class="empty">在上方选择一个任务以查看详情</p>

      <template v-else>
        <div v-show="tab === 'map'" ref="wrap" class="map">
          <canvas ref="canvas" />
          <p v-if="!detail?.blocks.length" class="empty small">暂无分块信息</p>
        </div>

        <div v-show="tab === 'jet'" class="jets">
          <div v-if="!jets.length" class="empty small">当前没有活跃连接</div>
          <div v-for="jet in jets" :key="jet.index" class="jet">
            <span class="idx">
              <i class="dot" :style="{ background: jetColor(jet.index) }" />#{{ jet.index + 1 }}
            </span>
            <span class="range mono">
              {{ formatBytes(jet.start) }} – {{ formatBytes(jet.end) }}
            </span>
            <span class="bar">
              <span
                class="fill"
                :style="{
                  width: `${percent(jet.done, jet.end - jet.start)}%`,
                  background: jetColor(jet.index),
                }"
              />
            </span>
            <span class="pct mono">
              {{ percent(jet.done, jet.end - jet.start).toFixed(0) }}%
            </span>
            <span class="speed mono">{{ formatSpeed(jet.speed) }}</span>
          </div>
        </div>

        <div v-show="tab === 'log'" ref="logBox" class="log">
          <p v-if="!log.length" class="empty small">暂无日志</p>
          <div v-for="(line, i) in log" :key="i" class="line">{{ line }}</div>
        </div>
      </template>
    </div>
  </div>
</template>

<style scoped>
.detail {
  display: flex;
  flex-direction: column;
  min-height: 0;
  height: 100%;
  background: var(--panel-bg);
  border-top: 1px solid var(--border);
}

.tabs {
  display: flex;
  align-items: center;
  gap: 2px;
  height: 21px;
  flex: 0 0 auto;
  padding: 0 6px;
  background: var(--panel-alt);
  border-bottom: 1px solid var(--border);
  font-size: var(--fs-small);
}

.tabs button {
  padding: 1px 9px;
  border: 1px solid transparent;
  border-bottom: 0;
  border-radius: var(--radius) var(--radius) 0 0;
  background: transparent;
  color: var(--text-dim);
  font: inherit;
  cursor: default;
}

.tabs button.on {
  background: var(--panel-bg);
  border-color: var(--border);
  color: var(--text);
}

.n {
  margin-left: 4px;
  color: var(--text-faint);
  font-variant-numeric: tabular-nums;
}

.grow {
  flex: 1;
}

.legend {
  display: flex;
  align-items: center;
  gap: 5px;
  color: var(--text-faint);
}

.legend .swatch {
  width: 8px;
  height: 8px;
  border-radius: 1px;
}

.legend .swatch.done {
  background: var(--progress-fill);
}

.legend .swatch.active {
  background: var(--accent);
}

.legend .swatch.idle {
  background: var(--progress-track);
}

.body {
  flex: 1;
  min-height: 0;
  overflow: auto;
  padding: 6px 8px;
}

.map {
  position: relative;
  min-height: 40px;
}

.map canvas {
  display: block;
}

.jets {
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.jet {
  display: flex;
  align-items: center;
  gap: 8px;
  height: 18px;
}

.idx {
  display: flex;
  align-items: center;
  gap: 4px;
  flex: 0 0 42px;
  color: var(--text-dim);
}

.dot {
  width: 7px;
  height: 7px;
  border-radius: 50%;
}

.range {
  flex: 0 0 170px;
  color: var(--text-dim);
}

.bar {
  flex: 1;
  height: 9px;
  border-radius: 2px;
  background: var(--progress-track);
  overflow: hidden;
}

.fill {
  display: block;
  height: 100%;
}

.pct {
  flex: 0 0 40px;
  text-align: right;
  color: var(--text-dim);
}

.speed {
  flex: 0 0 76px;
  text-align: right;
  color: var(--text);
}

.log {
  font-family: var(--font-mono);
  font-size: var(--fs-small);
  line-height: 1.55;
}

.line {
  white-space: pre-wrap;
  word-break: break-all;
}

.mono {
  font-family: var(--font-mono);
  font-variant-numeric: tabular-nums;
}

.empty {
  display: grid;
  place-items: center;
  height: 100%;
  min-height: 40px;
  margin: 0;
  color: var(--text-faint);
}

.empty.small {
  font-size: var(--fs-small);
}
</style>
