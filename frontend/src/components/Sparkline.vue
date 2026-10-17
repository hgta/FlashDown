<script setup lang="ts">
import { computed } from 'vue'

const props = withDefaults(
  defineProps<{ values: number[]; width?: number; height?: number }>(),
  { width: 72, height: 16 },
)

/**
 * A filled area sparkline over the rolling speed history. Scales to the window's
 * own maximum (not a fixed ceiling) so the shape stays legible at 10 KB/s and at
 * 10 MB/s alike.
 */
const path = computed(() => {
  const values = props.values?.length ? props.values : [0]
  const max = Math.max(1, ...values)
  const stepX = values.length > 1 ? props.width / (values.length - 1) : props.width

  const points = values.map((v, i) => {
    const x = i * stepX
    const y = props.height - (Math.max(0, v) / max) * (props.height - 2) - 1
    return `${x.toFixed(2)},${y.toFixed(2)}`
  })

  const line = `M${points.join(' L')}`
  const area = `${line} L${props.width},${props.height} L0,${props.height} Z`
  return { line, area }
})
</script>

<template>
  <svg
    class="spark"
    :width="width"
    :height="height"
    :viewBox="`0 0 ${width} ${height}`"
    preserveAspectRatio="none"
    aria-hidden="true"
  >
    <path :d="path.area" class="area" />
    <path :d="path.line" class="line" />
  </svg>
</template>

<style scoped>
.spark {
  display: block;
  overflow: visible;
}

.line {
  fill: none;
  stroke: var(--accent);
  stroke-width: 1.2;
  stroke-linejoin: round;
  stroke-linecap: round;
}

.area {
  fill: var(--accent);
  opacity: 0.18;
}
</style>
