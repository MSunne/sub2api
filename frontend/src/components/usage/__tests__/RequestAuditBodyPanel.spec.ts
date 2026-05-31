import { flushPromises, mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'

import RequestAuditBodyPanel from '../RequestAuditBodyPanel.vue'
import type { RequestAuditBody, RequestAuditLog } from '@/types'

const audit = {
  id: 42,
  request_id: 'req-42',
  request_body_kind: 'json',
  response_body_kind: 'sse',
  request_content_type: 'application/json',
  response_content_type: 'text/event-stream',
  request_truncated: false,
  response_truncated: false,
  request_bytes: 120,
  response_bytes: 220,
} as RequestAuditLog

describe('RequestAuditBodyPanel', () => {
  it('loads and expands audit bodies by default', async () => {
    const loadBody = vi.fn<[], Promise<RequestAuditBody>>().mockResolvedValue({
      role: 'request',
      body_kind: 'json',
      content_type: 'application/json',
      content_encoding: '',
      previewable: true,
      truncated: false,
      size_bytes: 120,
      body: '{"prompt":"hello"}',
    })

    const wrapper = mount(RequestAuditBodyPanel, {
      props: {
        audit,
        role: 'request',
        loadBody,
      },
    })

    await flushPromises()

    expect(loadBody).toHaveBeenCalledWith(42, 'request')
    expect(wrapper.text()).toContain('收起')
    expect(wrapper.text()).toContain('"prompt": "hello"')
  })

  it('keeps manual collapse state until the audit record changes', async () => {
    const loadBody = vi.fn<[], Promise<RequestAuditBody>>().mockResolvedValue({
      role: 'request',
      body_kind: 'json',
      content_type: 'application/json',
      content_encoding: '',
      previewable: true,
      truncated: false,
      size_bytes: 120,
      body: '{"prompt":"hello"}',
    })

    const wrapper = mount(RequestAuditBodyPanel, {
      props: {
        audit,
        role: 'request',
        loadBody,
      },
    })
    await flushPromises()

    await wrapper.get('button').trigger('click')
    expect(wrapper.text()).toContain('展开查看')

    await wrapper.setProps({ audit: { ...audit, duration_ms: 999 } })
    await flushPromises()

    expect(wrapper.text()).toContain('展开查看')

    await wrapper.setProps({ audit: { ...audit, id: 43, request_id: 'req-43' } })
    await flushPromises()

    expect(wrapper.text()).toContain('收起')
    expect(loadBody).toHaveBeenCalledWith(43, 'request')
  })

  it('renders request and response base64 images beside the raw audit body', async () => {
    const loadBody = vi.fn<[number, string], Promise<RequestAuditBody>>().mockResolvedValue({
      role: 'response',
      body_kind: 'sse',
      content_type: 'text/event-stream',
      content_encoding: '',
      previewable: true,
      truncated: false,
      size_bytes: 220,
      body: [
        'data: {"type":"image_edit.partial_image","b64_json":"iVBORw0KGgoAAAANSUhEUgAAAAEAAAAB"}',
        '',
        'data: {"type":"image_edit.completed","b64_json":"QUJDREVGR0hJSktMTU5PUA=="}',
        '',
      ].join('\n'),
    })

    const wrapper = mount(RequestAuditBodyPanel, {
      props: {
        audit,
        role: 'response',
        loadBody,
      },
    })

    await flushPromises()

    const images = wrapper.findAll('img')
    expect(images).toHaveLength(2)
    expect(images[0].attributes('src')).toContain('data:image/png;base64,iVBORw0KGgo')
    expect(images[1].attributes('src')).toContain('data:image/png;base64,QUJD')
    expect(wrapper.text()).toContain('"b64_json"')
  })
})
