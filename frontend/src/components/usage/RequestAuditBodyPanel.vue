<template>
  <section class="space-y-3 rounded border border-gray-200 p-3 dark:border-dark-600">
    <div class="flex flex-wrap items-center gap-3">
      <div class="min-w-0 flex-1">
        <div class="flex flex-wrap items-center gap-2 text-sm">
          <span class="font-medium text-gray-900 dark:text-white">{{ title }}</span>
          <span class="rounded bg-gray-100 px-2 py-0.5 font-mono text-xs text-gray-600 dark:bg-dark-700 dark:text-gray-300">
            {{ kindLabel }}
          </span>
          <span v-if="contentType" class="break-all rounded bg-gray-100 px-2 py-0.5 font-mono text-xs text-gray-600 dark:bg-dark-700 dark:text-gray-300">
            {{ contentType }}
          </span>
          <span v-if="truncated" class="rounded bg-amber-100 px-2 py-0.5 text-xs font-medium text-amber-700 dark:bg-amber-500/20 dark:text-amber-300">
            已截断
          </span>
        </div>
        <div class="mt-1 text-xs text-gray-500 dark:text-gray-400">
          原始大小 {{ formatBytes(sizeBytes) }}
        </div>
      </div>
      <button
        class="rounded border border-gray-200 px-3 py-1.5 text-xs font-medium text-gray-700 transition-colors hover:bg-gray-50 dark:border-dark-600 dark:text-gray-200 dark:hover:bg-dark-700"
        :disabled="loading"
        @click="toggle"
      >
        {{ expanded ? '收起' : '展开查看' }}
      </button>
    </div>

    <div v-if="expanded" class="space-y-3 border-t border-gray-200 pt-3 dark:border-dark-600">
      <div v-if="loading" class="py-6 text-center text-sm text-gray-500 dark:text-gray-400">加载中...</div>
      <div v-else-if="error" class="py-6 text-center text-sm text-gray-500 dark:text-gray-400">{{ error }}</div>
      <template v-else-if="body">
        <div v-if="!body.previewable" class="rounded bg-gray-50 p-4 text-sm text-gray-600 dark:bg-dark-800 dark:text-gray-300">
          该内容为二进制数据，不直接渲染。类型：{{ body.content_type || 'unknown' }}，大小：{{ formatBytes(body.size_bytes) }}。
        </div>

        <template v-else-if="body.body_kind === 'image'">
          <img :src="mediaDataUrl" class="max-h-[520px] max-w-full rounded border border-gray-200 object-contain dark:border-dark-600" alt="审计图片结果" />
        </template>

        <template v-else-if="body.body_kind === 'audio'">
          <audio :src="mediaDataUrl" controls class="w-full"></audio>
        </template>

        <template v-else-if="body.body_kind === 'sse'">
          <div class="space-y-2">
            <div v-for="(event, index) in sseEvents" :key="index" class="rounded border border-gray-200 bg-gray-50 p-3 dark:border-dark-600 dark:bg-dark-800">
              <div class="mb-2 flex items-center gap-2 text-xs text-gray-500 dark:text-gray-400">
                <span>#{{ index + 1 }}</span>
                <span v-if="event.event" class="rounded bg-gray-200 px-1.5 py-0.5 font-mono dark:bg-dark-700">{{ event.event }}</span>
              </div>
              <pre class="max-h-64 overflow-auto whitespace-pre-wrap break-words text-xs text-gray-800 dark:text-gray-100">{{ formatMaybeJSON(event.data) }}</pre>
            </div>
          </div>
        </template>

        <template v-else-if="body.body_kind === 'json'">
          <div v-if="mediaPreviews.length" class="grid grid-cols-1 gap-3 md:grid-cols-2">
            <div v-for="(media, index) in mediaPreviews" :key="index" class="rounded border border-gray-200 p-3 dark:border-dark-600">
              <div class="mb-2 text-xs font-medium text-gray-600 dark:text-gray-300">{{ media.label }}</div>
              <img v-if="media.type === 'image'" :src="media.src" class="max-h-72 max-w-full rounded object-contain" alt="JSON 内嵌图片" />
              <audio v-else :src="media.src" controls class="w-full"></audio>
            </div>
          </div>
          <pre class="max-h-[520px] overflow-auto whitespace-pre-wrap break-words rounded border border-gray-200 bg-gray-50 p-3 text-xs text-gray-800 dark:border-dark-600 dark:bg-dark-800 dark:text-gray-100">{{ formattedJSON }}</pre>
        </template>

        <pre v-else class="max-h-[520px] overflow-auto whitespace-pre-wrap break-words rounded border border-gray-200 bg-gray-50 p-3 text-xs text-gray-800 dark:border-dark-600 dark:bg-dark-800 dark:text-gray-100">{{ body.body }}</pre>
      </template>
    </div>
  </section>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import type { RequestAuditBody, RequestAuditBodyRole, RequestAuditLog } from '@/types'

interface Props {
  audit: RequestAuditLog
  role: RequestAuditBodyRole
  loadBody: (auditId: number, role: RequestAuditBodyRole) => Promise<RequestAuditBody>
}

const props = defineProps<Props>()

const expanded = ref(false)
const loading = ref(false)
const error = ref('')
const body = ref<RequestAuditBody | null>(null)

const title = computed(() => props.role === 'request' ? '提交参数' : '运行结果')
const contentType = computed(() => props.role === 'request' ? props.audit.request_content_type : props.audit.response_content_type)
const kind = computed(() => props.role === 'request' ? props.audit.request_body_kind : props.audit.response_body_kind)
const kindLabel = computed(() => kind.value || 'unknown')
const truncated = computed(() => props.role === 'request' ? props.audit.request_truncated : props.audit.response_truncated)
const sizeBytes = computed(() => props.role === 'request' ? props.audit.request_bytes : props.audit.response_bytes)

watch(
  () => [props.audit.id, props.role],
  () => {
    expanded.value = false
    loading.value = false
    error.value = ''
    body.value = null
  }
)

const toggle = async () => {
  if (expanded.value) {
    expanded.value = false
    return
  }
  expanded.value = true
  if (!body.value) {
    await loadBody()
  }
}

const loadBody = async () => {
  loading.value = true
  error.value = ''
  try {
    body.value = await props.loadBody(props.audit.id, props.role)
  } catch {
    error.value = '未采集、已清理或内容不可用'
  } finally {
    loading.value = false
  }
}

const mediaDataUrl = computed(() => {
  if (!body.value) return ''
  if (body.value.body.startsWith('data:')) return body.value.body
  const mime = body.value.media_mime || body.value.content_type || (body.value.body_kind === 'audio' ? 'audio/mpeg' : 'image/png')
  return body.value.content_encoding === 'base64'
    ? `data:${mime};base64,${body.value.body}`
    : body.value.body
})

const parsedJSON = computed(() => {
  if (!body.value || body.value.body_kind !== 'json') return null
  try {
    return JSON.parse(body.value.body)
  } catch {
    return null
  }
})

const formattedJSON = computed(() => {
  if (!body.value) return ''
  if (!parsedJSON.value) return body.value.body
  return JSON.stringify(summarizeLargeJSONStrings(parsedJSON.value), null, 2)
})

interface MediaPreview {
  type: 'image' | 'audio'
  src: string
  label: string
}

const mediaPreviews = computed<MediaPreview[]>(() => {
  if (!parsedJSON.value) return []
  const previews: MediaPreview[] = []
  collectJSONMedia(parsedJSON.value, previews, 'root')
  return previews.slice(0, 12)
})

const sseEvents = computed(() => {
  if (!body.value) return []
  return parseSSE(body.value.body)
})

const formatBytes = (bytes: number) => {
  if (!bytes) return '0 B'
  if (bytes < 1024) return `${bytes} B`
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`
  return `${(bytes / 1024 / 1024).toFixed(2)} MB`
}

const formatMaybeJSON = (value: string) => {
  try {
    return JSON.stringify(JSON.parse(value), null, 2)
  } catch {
    return value
  }
}

const parseSSE = (raw: string) => raw
  .split(/\n\n+/)
  .map((chunk) => {
    const event = { event: '', data: '' }
    chunk.split(/\r?\n/).forEach((line) => {
      if (line.startsWith('event:')) event.event = line.slice(6).trim()
      if (line.startsWith('data:')) event.data += `${event.data ? '\n' : ''}${line.slice(5).trim()}`
    })
    return event
  })
  .filter((event) => event.event || event.data)

const dataURLPattern = /^data:(image|audio)\/[^;]+;base64,/i

function collectJSONMedia(value: unknown, previews: MediaPreview[], path: string): void {
  if (previews.length >= 12 || value == null) return
  if (typeof value === 'string') {
    const match = value.match(dataURLPattern)
    if (match) {
      previews.push({ type: match[1].toLowerCase() === 'audio' ? 'audio' : 'image', src: value, label: path })
    }
    return
  }
  if (Array.isArray(value)) {
    value.forEach((item, index) => collectJSONMedia(item, previews, `${path}[${index}]`))
    return
  }
  if (typeof value === 'object') {
    const record = value as Record<string, unknown>
    const mime = typeof record.mime_type === 'string' ? record.mime_type : ''
    if (typeof record.b64_json === 'string') {
      previews.push({ type: 'image', src: `data:${mime || 'image/png'};base64,${record.b64_json}`, label: `${path}.b64_json` })
    }
    if (typeof record.data === 'string' && /^image\/|^audio\//.test(mime)) {
      previews.push({ type: mime.startsWith('audio/') ? 'audio' : 'image', src: `data:${mime};base64,${record.data}`, label: `${path}.data` })
    }
    Object.entries(record).forEach(([key, child]) => collectJSONMedia(child, previews, `${path}.${key}`))
  }
}

function summarizeLargeJSONStrings(value: unknown): unknown {
  if (typeof value === 'string') {
    const dataMatch = value.match(dataURLPattern)
    if (dataMatch) return `[${dataMatch[1]} data URL, ${value.length} chars]`
    if (/^[A-Za-z0-9+/=_-]{2048,}$/.test(value)) return `[large encoded string, ${value.length} chars]`
    if (value.length > 8000) return `${value.slice(0, 8000)}\n...[truncated in viewer, ${value.length} chars total]`
    return value
  }
  if (Array.isArray(value)) return value.map(summarizeLargeJSONStrings)
  if (value && typeof value === 'object') {
    return Object.fromEntries(
      Object.entries(value as Record<string, unknown>).map(([key, child]) => [key, summarizeLargeJSONStrings(child)])
    )
  }
  return value
}
</script>
