<script setup lang="ts">
import {
  categoryCounts,
  counts,
  openFolder,
  selectFilter,
  state,
  STATUS_NODES,
} from '../stores/session'

function isActive(kind: 'status' | 'category', id: string): boolean {
  return state.filter.kind === kind && state.filter.id === id
}
</script>

<template>
  <div class="tree">
    <div class="section">
      <button
        v-for="node in STATUS_NODES"
        :key="node.id"
        type="button"
        class="row"
        :class="{ active: isActive('status', node.id) }"
        @click="selectFilter('status', node.id)"
      >
        <span class="glyph" :class="node.id" />
        <span class="name">{{ node.name }}</span>
        <span class="count">{{ counts[node.id] }}</span>
      </button>
    </div>

    <div class="divider">
      <span>类别</span>
      <button class="mini" type="button" disabled title="类别管理（M3）">＋</button>
    </div>

    <div class="section scroll">
      <button
        v-for="cat in state.config?.categories ?? []"
        :key="cat.id"
        type="button"
        class="row"
        :class="{ active: isActive('category', cat.id) }"
        :title="cat.dir"
        @click="selectFilter('category', cat.id)"
        @dblclick="openFolder(cat.dir)"
      >
        <span class="glyph folder" />
        <span class="name">{{ cat.name }}</span>
        <span class="count">{{ categoryCounts[cat.id] ?? 0 }}</span>
      </button>
    </div>
  </div>
</template>

<style scoped>
.tree {
  display: flex;
  flex-direction: column;
  min-height: 0;
  background: var(--panel-alt);
  border-right: 1px solid var(--border);
  overflow: hidden;
}

.section {
  display: flex;
  flex-direction: column;
  padding: 3px 0;
}

.section.scroll {
  overflow-y: auto;
  flex: 1;
  min-height: 0;
}

.row {
  display: flex;
  align-items: center;
  gap: 6px;
  height: 22px;
  padding: 0 8px 0 10px;
  border: 0;
  background: transparent;
  color: var(--text);
  font: inherit;
  text-align: left;
  cursor: default;
}

.row:hover {
  background: var(--row-hover);
}

.row.active {
  background: var(--row-selected);
  box-shadow: inset 0 0 0 1px var(--row-selected-border);
}

.glyph {
  width: 10px;
  height: 10px;
  flex: 0 0 10px;
  border-radius: 2px;
  background: var(--text-faint);
}

/* Status nodes get FlashGet-ish signal colours. */
.glyph.downloading {
  background: var(--accent);
}

.glyph.finished {
  background: var(--ok);
}

.glyph.removed {
  background: var(--text-faint);
}

.glyph.folder {
  background: var(--warn);
  border-radius: 1px 2px 2px 2px;
}

.name {
  flex: 1;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.count {
  color: var(--text-faint);
  font-size: var(--fs-small);
  font-variant-numeric: tabular-nums;
}

.divider {
  display: flex;
  align-items: center;
  justify-content: space-between;
  height: 20px;
  padding: 0 6px 0 10px;
  border-top: 1px solid var(--border);
  border-bottom: 1px solid var(--border);
  background: var(--panel-sunken);
  color: var(--text-dim);
  font-size: var(--fs-small);
  letter-spacing: 0.3px;
}

.mini {
  border: 0;
  background: transparent;
  color: var(--text-faint);
  font: inherit;
  line-height: 1;
  cursor: default;
}
</style>
