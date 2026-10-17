/**
 * Theme resolution.
 *
 * The config offers system / light / dark. Rather than duplicating a dark token
 * block behind both a media query and an attribute selector, we resolve the
 * effective theme here and always write `data-theme="light|dark"` onto <html>.
 * tokens.css then needs exactly one dark block.
 */

import type { UIConfig } from '../types'

export type ThemeMode = UIConfig['theme']

let current: ThemeMode = 'system'
let media: MediaQueryList | null = null

function systemPrefersDark(): boolean {
  return media ? media.matches : false
}

function resolve(): void {
  const dark = current === 'dark' || (current === 'system' && systemPrefersDark())
  document.documentElement.dataset.theme = dark ? 'dark' : 'light'
}

/** Applies a mode and keeps following the OS while the mode is `system`. */
export function applyTheme(mode: ThemeMode): void {
  current = mode

  if (!media) {
    media = window.matchMedia('(prefers-color-scheme: dark)')
    media.addEventListener('change', () => {
      if (current === 'system') resolve()
    })
  }
  resolve()
}

/** The theme actually in effect right now, for UI that needs to know. */
export function effectiveTheme(): 'light' | 'dark' {
  return document.documentElement.dataset.theme === 'dark' ? 'dark' : 'light'
}
