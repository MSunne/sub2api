import { describe, expect, it } from 'vitest'

import { extractBase64ImagesFromAuditContent } from '../requestAuditImages'

const pngA = 'iVBORw0KGgoAAAANSUhEUgAAAAEAAAAB'
const pngB = 'QUJDREVGR0hJSktMTU5PUA=='

describe('extractBase64ImagesFromAuditContent', () => {
  it('extracts image base64 values from nested JSON payloads', () => {
    const images = extractBase64ImagesFromAuditContent(
      JSON.stringify({
        model: 'gpt-image-2',
        input: [
          {
            type: 'input_image',
            mime_type: 'image/jpeg',
            image_base64: pngA,
          },
        ],
        data: [
          {
            b64_json: pngB,
          },
        ],
      }),
      'request'
    )

    expect(images).toHaveLength(2)
    expect(images[0]).toMatchObject({
      source: 'request',
      path: 'root.input[0].image_base64',
      mimeType: 'image/jpeg',
      dataUrl: `data:image/jpeg;base64,${pngA}`,
    })
    expect(images[1]).toMatchObject({
      source: 'request',
      path: 'root.data[0].b64_json',
      mimeType: 'image/png',
      dataUrl: `data:image/png;base64,${pngB}`,
    })
  })

  it('extracts image base64 values from SSE event data', () => {
    const images = extractBase64ImagesFromAuditContent(
      [
        `event: image_edit.partial_image`,
        `data: {"type":"image_edit.partial_image","b64_json":"${pngA}"}`,
        '',
        `data: {"type":"image_edit.completed","b64_json":"${pngB}","output_format":"webp"}`,
        '',
      ].join('\n'),
      'response'
    )

    expect(images).toHaveLength(2)
    expect(images.map((image) => image.label)).toEqual([
      '#1 image_edit.partial_image root.b64_json',
      '#2 image_edit.completed root.b64_json',
    ])
    expect(images[1].mimeType).toBe('image/webp')
    expect(images[1].dataUrl).toBe(`data:image/webp;base64,${pngB}`)
  })

  it('ignores truncated or non-image encoded strings and de-duplicates repeated data URLs', () => {
    const dataUrl = `data:image/png;base64,${pngA}`
    const images = extractBase64ImagesFromAuditContent(
      {
        first: dataUrl,
        repeat: dataUrl,
        plain: 'not-image-text',
        truncated: `${pngB}...[truncated]`,
        audio: {
          mime_type: 'audio/mpeg',
          base64: pngB,
        },
      },
      'response'
    )

    expect(images).toHaveLength(1)
    expect(images[0]).toMatchObject({
      path: 'root.first',
      mimeType: 'image/png',
      dataUrl,
    })
  })
})
