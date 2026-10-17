<script setup lang="ts">
/**
 * The new-task dialog (2.13).
 *
 * One link per dialog on purpose: batch URLs get their own preview dialog in §4.9,
 * where the expanded list can be reviewed before anything is queued. Pretending
 * this dialog handles batches would just postpone that work.
 */
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { addTask, closeTaskDialog, pickFolder, state } from '../stores/session'

const url = ref(state.dialogUrl)
const name = ref('')
const comment = ref('')
const startNow = ref(true)
const category = ref(state.config?.categories?.[0]?.id ?? '')
const dir = ref('')
const localError = ref('')
const urlInput = ref<HTMLInputElement | null>(null)

const categories = computed(() => state.config?.categories ?? [])
const error = computed(() => localError.value || state.dialogError)

function dirOfCategory(id: string): string {
  const cat = categories.value.find((c) => c.id === id)
  return cat?.dir || state.config?.downloadRoot || ''
}

// The category *is* the directory, so picking one seeds the path. Editing the path
// afterwards is allowed; it simply stops following the category.
dir.value = dirOfCategory(category.value)
watch(category, (id) => {
  dir.value = dirOfCategory(id)
})

onMounted(() => {
  urlInput.value?.focus()
  document.addEventListener('keydown', onKey)
})
onBeforeUnmount(() => document.removeEventListener('keydown', onKey))

function onKey(event: KeyboardEvent) {
  if (event.key === 'Escape') {
    event.preventDefault()
    closeTaskDialog()
  }
}

/** Client-side checks only catch the obvious; the engine owns the real validation. */
function validate(): boolean {
  const value = url.value.trim()
  if (!value) {
    localError.value = '请填写下载链接'
    return false
  }
  try {
    const parsed = new URL(value)
    if (parsed.protocol !== 'http:' && parsed.protocol !== 'https:') {
      localError.value = '只支持 http / https 链接'
      return false
    }
  } catch {
    localError.value = '链接格式不正确'
    return false
  }
  if (!dir.value.trim()) {
    localError.value = '请选择保存目录'
    return false
  }
  return true
}

async function submit() {
  localError.value = ''
  if (!validate()) return

  await addTask({
    url: url.value.trim(),
    name: name.value.trim(),
    dir: dir.value.trim(),
    category: category.value,
    comment: comment.value.trim(),
    start: startNow.value,
  })
}

async function browse() {
  const picked = await pickFolder(dir.value)
  if (picked) dir.value = picked
}
</script>

<template>
  <div class="overlay" @mousedown.self="closeTaskDialog()">
    <form class="dialog" @submit.prevent="submit">
      <header>
        <h2>新建下载任务</h2>
        <button type="button" class="close" title="关闭" @click="closeTaskDialog()">✕</button>
      </header>

      <div class="body">
        <label class="row">
          <span class="key">下载链接</span>
          <input
            ref="urlInput"
            v-model="url"
            type="text"
            spellcheck="false"
            placeholder="http:// 或 https://"
          />
        </label>

        <label class="row">
          <span class="key">另存为</span>
          <input v-model="name" type="text" placeholder="留空则使用服务器提供的文件名" />
        </label>

        <label class="row">
          <span class="key">保存到</span>
          <span class="with-button">
            <input v-model="dir" type="text" spellcheck="false" />
            <button type="button" class="browse" @click="browse">浏览…</button>
          </span>
        </label>

        <label class="row">
          <span class="key">类别</span>
          <select v-model="category">
            <option value="">（不归类）</option>
            <option v-for="cat in categories" :key="cat.id" :value="cat.id">{{ cat.name }}</option>
          </select>
        </label>

        <label class="row">
          <span class="key">注释</span>
          <input v-model="comment" type="text" placeholder="可选" />
        </label>

        <!-- Being explicit about the gap beats three greyed-out inputs with no
             explanation. Block count lands with M2, the HTTP options after that. -->
        <p class="later">块数、引用页、登录：随 M2 的多线程内核提供</p>

        <div class="row">
          <span class="key">开始方式</span>
          <span class="choices">
            <label class="choice">
              <input v-model="startNow" type="radio" :value="true" />
              立即开始
            </label>
            <label class="choice">
              <input v-model="startNow" type="radio" :value="false" />
              仅添加到列表
            </label>
          </span>
        </div>
      </div>

      <footer>
        <span class="error" :title="error">{{ error }}</span>
        <button type="button" class="btn" @click="closeTaskDialog()">取消</button>
        <button type="submit" class="btn primary" :disabled="state.dialogBusy">
          {{ state.dialogBusy ? '正在添加…' : '确定' }}
        </button>
      </footer>
    </form>
  </div>
</template>

<style scoped>
.overlay {
  position: fixed;
  inset: 0;
  z-index: 200;
  display: grid;
  place-items: center;
  background: rgb(10 14 22 / 42%);
}

.dialog {
  width: 520px;
  max-width: calc(100vw - 40px);
  background: var(--panel-bg);
  border: 1px solid var(--border-strong);
  border-radius: 4px;
  box-shadow: var(--shadow-popup);
  color: var(--text);
  font: var(--fs-body) var(--font-ui);
}

header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  height: 28px;
  padding: 0 8px 0 12px;
  background: linear-gradient(var(--header-top), var(--header-bottom));
  border-bottom: 1px solid var(--border);
}

h2 {
  margin: 0;
  font-size: var(--fs-title);
  font-weight: 600;
}

.close {
  width: 20px;
  height: 20px;
  border: 0;
  border-radius: var(--radius);
  background: transparent;
  color: var(--text-dim);
  font: inherit;
  line-height: 1;
  cursor: default;
}

.close:hover {
  background: var(--danger);
  color: var(--text-invert);
}

.body {
  display: flex;
  flex-direction: column;
  gap: 7px;
  padding: 12px 12px 10px;
}

.row {
  display: flex;
  align-items: center;
  gap: 8px;
}

.key {
  flex: 0 0 62px;
  color: var(--text-dim);
  text-align: right;
}

input[type='text'],
select {
  flex: 1;
  min-width: 0;
  height: 22px;
  padding: 0 6px;
  border: 1px solid var(--border);
  border-radius: var(--radius);
  background: var(--panel-sunken);
  color: var(--text);
  font: inherit;
  outline: none;
}

input[type='text']:focus,
select:focus {
  border-color: var(--accent);
  background: var(--panel-bg);
  box-shadow: var(--focus);
}

input::placeholder {
  color: var(--text-faint);
}

.with-button {
  display: flex;
  flex: 1;
  gap: 4px;
  min-width: 0;
}

.browse {
  flex: 0 0 auto;
  height: 22px;
  padding: 0 10px;
  border: 1px solid var(--border);
  border-radius: var(--radius);
  background: var(--panel-alt);
  color: var(--text);
  font: inherit;
  cursor: default;
}

.browse:hover {
  border-color: var(--accent);
  background: var(--accent-soft);
}

.later {
  margin: 0 0 0 70px;
  color: var(--text-faint);
  font-size: var(--fs-small);
}

.choices {
  display: flex;
  gap: 14px;
}

.choice {
  display: flex;
  align-items: center;
  gap: 4px;
}

.choice input {
  margin: 0;
}

footer {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 8px 12px;
  border-top: 1px solid var(--border);
  background: var(--panel-alt);
}

.error {
  flex: 1;
  color: var(--danger);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.btn {
  height: 23px;
  min-width: 68px;
  padding: 0 12px;
  border: 1px solid var(--border-strong);
  border-radius: var(--radius);
  background: var(--panel-bg);
  color: var(--text);
  font: inherit;
  cursor: default;
}

.btn:hover {
  border-color: var(--accent);
}

.btn.primary {
  border-color: var(--accent);
  background: var(--accent);
  color: var(--text-invert);
}

.btn.primary:hover {
  background: var(--accent-hover);
}

.btn:disabled {
  opacity: 0.6;
}
</style>
