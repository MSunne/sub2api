export interface AuditImagePreview {
  id: string
  source: string
  path: string
  mimeType: string
  dataUrl: string
  label: string
}

interface ExtractContext {
  source: string
  seen: Set<string>
  out: AuditImagePreview[]
}

const dataImagePattern = /^data:(image\/[a-z0-9.+-]+);base64,([a-z0-9+/=_-]+)$/i
const base64Pattern = /^[A-Za-z0-9+/=_-]{16,}$/
const imageBase64Keys = new Set(['b64_json', 'image_base64', 'base64'])

export function extractBase64ImagesFromAuditContent(content: unknown, source: string): AuditImagePreview[] {
  const ctx: ExtractContext = { source, seen: new Set(), out: [] }
  collectFromUnknown(resolveContent(content), 'root', ctx, '')
  return ctx.out
}

function resolveContent(content: unknown): unknown {
  if (typeof content !== 'string') return content
  const trimmed = content.trim()
  if (!trimmed) return content
  if (looksLikeSSE(trimmed)) return parseSSE(trimmed)
  try {
    return JSON.parse(trimmed)
  } catch {
    return content
  }
}

function looksLikeSSE(raw: string): boolean {
  return /(^|\n)(data|event):/.test(raw)
}

function parseSSE(raw: string): Array<{ event: string; data: string; index: number }> {
  return raw
    .split(/\n\s*\n+/)
    .map((chunk, index) => {
      const event = { event: '', data: '', index: index + 1 }
      for (const line of chunk.split(/\r?\n/)) {
        if (line.startsWith('event:')) event.event = line.slice(6).trim()
        if (line.startsWith('data:')) event.data += `${event.data ? '\n' : ''}${line.slice(5).trim()}`
      }
      return event
    })
    .filter((event) => event.event || event.data)
}

function collectFromUnknown(value: unknown, path: string, ctx: ExtractContext, labelPrefix: string): void {
  if (ctx.out.length >= 24 || value == null) return

  if (typeof value === 'string') {
    addDataUrl(value, path, ctx, labelPrefix)
    return
  }

  if (Array.isArray(value)) {
    value.forEach((item, index) => {
      if (isSSEEvent(item)) {
        collectFromSSEEvent(item, ctx)
      } else {
        collectFromUnknown(item, `${path}[${index}]`, ctx, labelPrefix)
      }
    })
    return
  }

  if (typeof value !== 'object') return

  const record = value as Record<string, unknown>
  for (const [key, child] of Object.entries(record)) {
    const childPath = `${path}.${key}`
    if (typeof child === 'string' && imageBase64Keys.has(key)) {
      const mimeType = resolveMimeType(record, key)
      if (mimeType.startsWith('image/') && isUsableBase64(child)) {
        addPreview(childPath, mimeType, `data:${mimeType};base64,${child}`, ctx, labelPrefix)
      }
    }
    collectFromUnknown(child, childPath, ctx, labelPrefix)
  }
}

function collectFromSSEEvent(event: { event: string; data: string; index: number }, ctx: ExtractContext): void {
  const data = event.data.trim()
  if (!data || data === '[DONE]') return

  let parsed: unknown = data
  try {
    parsed = JSON.parse(data)
  } catch {
    parsed = data
  }

  const eventType = typeof parsed === 'object' && parsed !== null && typeof (parsed as Record<string, unknown>).type === 'string'
    ? String((parsed as Record<string, unknown>).type)
    : event.event
  const prefix = `#${event.index}${eventType ? ` ${eventType}` : ''} `
  collectFromUnknown(parsed, 'root', ctx, prefix)
}

function isSSEEvent(value: unknown): value is { event: string; data: string; index: number } {
  return Boolean(
    value &&
    typeof value === 'object' &&
    'data' in value &&
    'index' in value &&
    typeof (value as { data: unknown }).data === 'string'
  )
}

function addDataUrl(value: string, path: string, ctx: ExtractContext, labelPrefix: string): void {
  const match = value.match(dataImagePattern)
  if (!match) return
  addPreview(path, match[1].toLowerCase(), value, ctx, labelPrefix)
}

function addPreview(path: string, mimeType: string, dataUrl: string, ctx: ExtractContext, labelPrefix: string): void {
  if (ctx.seen.has(dataUrl)) return
  ctx.seen.add(dataUrl)
  const label = `${labelPrefix}${path}`.trim()
  ctx.out.push({
    id: `${ctx.source}:${ctx.out.length}:${path}`,
    source: ctx.source,
    path,
    mimeType,
    dataUrl,
    label,
  })
}

function resolveMimeType(record: Record<string, unknown>, key: string): string {
  const explicit = firstString(record.mime_type, record.media_type, record.content_type)
  if (explicit && explicit.startsWith('image/')) return explicit

  const outputFormat = firstString(record.output_format, record.format)
  if (outputFormat) {
    const normalized = outputFormat.toLowerCase().replace(/^image\//, '')
    if (/^[a-z0-9.+-]+$/.test(normalized)) return `image/${normalized === 'jpg' ? 'jpeg' : normalized}`
  }

  return key === 'base64' ? '' : 'image/png'
}

function firstString(...values: unknown[]): string {
  for (const value of values) {
    if (typeof value === 'string' && value.trim()) return value.trim().toLowerCase()
  }
  return ''
}

function isUsableBase64(value: string): boolean {
  const trimmed = value.trim()
  return base64Pattern.test(trimmed) && !trimmed.includes('...[truncated]') && !trimmed.includes('[truncated]')
}
