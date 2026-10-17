/**
 * Drag-and-drop plumbing shared by the two drop baskets (design.md D12).
 *
 * Only the HTML5 path lives here: a drag out of a browser carries
 * `text/uri-list` and is an ordinary DOM drop. Explorer file drags never reach
 * the DOM at all - Wails intercepts them natively and Go forwards them as
 * `files:dropped`, which the store handles. Mixing the two in one handler is the
 * trap this split exists to avoid.
 *
 * The caller supplies what to do with a captured link, because the two windows
 * answer differently: the main window opens its own dialog, the floating window
 * has to ask the main window to open one.
 */
import { ref } from 'vue'
import { firstHttpUrl } from './url'
import { rejectDrop } from '../stores/session'

export function useDropBasket(onUrl: (url: string) => void) {
  const over = ref(false)

  function onDragOver(event: DragEvent) {
    // Without claiming dragover the WebView navigates to the dropped link.
    event.preventDefault()
    if (event.dataTransfer) event.dataTransfer.dropEffect = 'link'
    over.value = true
  }

  function onDragLeave() {
    over.value = false
  }

  function onDrop(event: DragEvent) {
    event.preventDefault()
    over.value = false

    const dt = event.dataTransfer
    if (!dt) return
    // text/uri-list is what a browser actually sends; text/plain is the fallback
    // for editors and bare-text drags.
    const url = firstHttpUrl(dt.getData('text/uri-list') || dt.getData('text/plain') || '')
    if (!url) {
      rejectDrop()
      return
    }
    onUrl(url)
  }

  return { over, onDragOver, onDragLeave, onDrop }
}
