/**
 * Link extraction for the drop baskets.
 *
 * A drag from a browser carries `text/uri-list` (one URI per line, `#` starts a
 * comment) and usually `text/plain` as well. Both are plain text, so a single
 * line-oriented scan handles either - and anything that fails to parse as an
 * http(s) URL is reported as "not a link" rather than guessed at.
 */
export function firstHttpUrl(text: string): string {
  if (!text) return ''

  for (const raw of text.split(/[\r\n]+/)) {
    const candidate = raw.trim()
    if (!candidate || candidate.startsWith('#')) continue
    try {
      const url = new URL(candidate)
      if (url.protocol === 'http:' || url.protocol === 'https:') return url.toString()
    } catch {
      // Not a URL on its own - keep looking, the payload may hold several lines.
    }
  }
  return ''
}
