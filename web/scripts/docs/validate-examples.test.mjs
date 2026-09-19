/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import test from 'node:test'

import { relayOpenAPIPath } from './common.mjs'
import {
  validateMarkdownJSONExamples,
  validateOpenAPIExamples,
} from './validate-examples.mjs'

const openapi = JSON.parse(readFileSync(relayOpenAPIPath, 'utf8'))

function responseExample(schema, value) {
  return {
    openapi: '3.0.1',
    components: structuredClone(openapi.components),
    paths: {
      '/example': {
        get: {
          operationId: 'example',
          responses: {
            200: {
              content: {
                'application/json': {
                  schema: { $ref: `#/components/schemas/${schema}` },
                  example: value,
                },
              },
            },
          },
        },
      },
    },
  }
}

const approved = new Set(['example'])

test('published JSON edit examples match their declared native or unified contract', () => {
  const page = readFileSync(
    new URL(
      '../../public/docs-content/zh/api-reference/images/edits.md',
      import.meta.url
    ),
    'utf8'
  )
  for (const [heading, schema] of [
    ['### 原生 OpenAI JSON 编辑：对象数组', 'NativeImageEditJSONRequest'],
    ['### 统一图片适配：字符串引用', 'UnifiedImageEditJSONRequest'],
  ]) {
    const start = page.indexOf(heading)
    assert.ok(start >= 0)
    const end = page.indexOf('\n##', start + heading.length)
    const section = page.slice(start, end < 0 ? undefined : end)
    let count = 0
    for (const [, language, body] of section.matchAll(
      /^```(json|bash)\n([\s\S]*?)^```/gm
    )) {
      const values =
        language === 'json'
          ? [body]
          : [...body.matchAll(/-d\s+'([^']*)'/g)].map((match) => match[1])
      for (const value of values) {
        assert.doesNotThrow(() =>
          validateOpenAPIExamples(
            responseExample(schema, JSON.parse(value)),
            approved
          )
        )
        count += 1
      }
    }
    assert.ok(count > 0, `${schema} needs a published example`)
  }
})

test('native JSON edits require ordered reference objects and keep unified strings separate', () => {
  const base = { model: 'customer-image-model', prompt: 'Red cup' }
  const image = { image_url: 'https://example.com/cup.png' }
  for (const value of [
    { ...base, images: [image] },
    { ...base, images: [image], response_format: 'url' },
    { ...base, images: [image, { file_id: 'file-example' }], mask: image },
    { ...base, images: Array(16).fill(image), quality: 'max' },
  ]) {
    assert.doesNotThrow(() =>
      validateOpenAPIExamples(
        responseExample('NativeImageEditJSONRequest', value),
        approved
      )
    )
    assert.doesNotThrow(() =>
      validateOpenAPIExamples(
        responseExample('ImageEditJSONRequest', value),
        approved
      )
    )
    assert.throws(
      () =>
        validateOpenAPIExamples(
          responseExample('UnifiedImageEditJSONRequest', value),
          approved
        ),
      /示例不符合合同/
    )
  }
  for (const value of [
    { ...base, image: image.image_url },
    { ...base, images: [image.image_url] },
    { ...base, images: [] },
    { ...base, images: [image], image: image.image_url },
    { ...base, images: [image, image.image_url] },
    { ...base, images: [{ image_url: { url: image.image_url } }] },
    { ...base, images: [{}] },
    { ...base, images: [{ ...image, file_id: 'file-example' }] },
    { ...base, images: Array(17).fill(image) },
    { ...base, images: [image], mask: image.image_url },
    { ...base, images: [image], response_format: 'invalid-format' },
    { ...base, images: [image], n: 11 },
  ]) {
    assert.throws(
      () =>
        validateOpenAPIExamples(
          responseExample('NativeImageEditJSONRequest', value),
          approved
        ),
      /示例不符合合同/
    )
  }
  assert.throws(
    () =>
      validateOpenAPIExamples(
        responseExample('ImageEditJSONRequest', {
          ...base,
          images: [image, image.image_url],
        }),
        approved
      ),
    /示例不符合合同/
  )
})

test('multipart image edits accept one file field convention', () => {
  const base = { model: 'customer-image-model', prompt: 'Red cup' }
  for (const value of [
    { ...base, image: 'file bytes' },
    { ...base, 'image[]': ['first', 'second'] },
  ]) {
    assert.doesNotThrow(() =>
      validateOpenAPIExamples(
        responseExample('ImageEditRequest', value),
        approved
      )
    )
  }
  assert.throws(
    () =>
      validateOpenAPIExamples(
        responseExample('ImageEditRequest', {
          ...base,
          image: 'first',
          'image[]': ['second'],
        }),
        approved
      ),
    /示例不符合合同/
  )
})

const edit = {
  model: 'customer-image-model',
  prompt: 'Red cup',
  image: 'https://example.com/cup.png',
}

test('JSON edit examples allow one image or ordered images, not both or neither', () => {
  for (const value of [
    edit,
    { model: edit.model, prompt: edit.prompt, images: [edit.image] },
  ]) {
    assert.doesNotThrow(() =>
      validateOpenAPIExamples(
        responseExample('ImageEditJSONRequest', value),
        approved
      )
    )
  }
  for (const value of [
    { ...edit, images: [edit.image] },
    { model: edit.model, prompt: edit.prompt },
    { ...edit, n: '1' },
  ]) {
    assert.throws(
      () =>
        validateOpenAPIExamples(
          responseExample('ImageEditJSONRequest', value),
          approved
        ),
      /示例不符合合同/
    )
  }
})

test('image examples reject unpublished extra fields and an excessive count', () => {
  const request = {
    model: edit.model,
    prompt: edit.prompt,
    extra_fields: { aspect_ratio: '16:9' },
  }
  assert.doesNotThrow(() =>
    validateOpenAPIExamples(
      responseExample('ImageGenerationRequest', request),
      approved
    )
  )
  for (const value of [
    { ...request, n: 129 },
    { ...request, extra_fields: { private_option: 'value' } },
  ]) {
    assert.throws(
      () =>
        validateOpenAPIExamples(
          responseExample('ImageGenerationRequest', value),
          approved
        ),
      /示例不符合合同/
    )
  }
})

test('image task failures use a direct error object, not a nested HTTP envelope', () => {
  const task = {
    id: 'task-example',
    object: 'image_task',
    status: 'failed',
    error: { code: 'generation_failed', message: 'Failed' },
  }
  assert.doesNotThrow(() =>
    validateOpenAPIExamples(responseExample('ImageTaskStatus', task), approved)
  )
  assert.throws(
    () =>
      validateOpenAPIExamples(
        responseExample('ImageTaskStatus', {
          ...task,
          error: { error: task.error },
        }),
        approved
      ),
    /示例不符合合同/
  )
})

test('generic video query examples require the data envelope', () => {
  const task = {
    task_id: 'task-example',
    status: 'succeeded',
    url: 'https://example.com/video.mp4',
    metadata: null,
    error: null,
  }
  assert.doesNotThrow(() =>
    validateOpenAPIExamples(
      responseExample('VideoTaskResponse', {
        code: 'success',
        message: '',
        data: task,
      }),
      approved
    )
  )
  assert.throws(
    () =>
      validateOpenAPIExamples(
        responseExample('VideoTaskResponse', task),
        approved
      ),
    /示例不符合合同/
  )
})

test('ModelArk media examples require the matching role and exactly one payload', () => {
  const image = {
    type: 'image_url',
    role: 'first_frame',
    image_url: { url: 'https://example.com/frame.png' },
  }
  const video = {
    type: 'video_url',
    role: 'reference_video',
    video_url: { url: 'https://example.com/reference.mp4' },
  }
  const audio = {
    type: 'audio_url',
    role: 'reference_audio',
    audio_url: { url: 'https://example.com/reference.wav' },
  }
  const text = { type: 'text', text: 'A blue cup' }
  for (const content of [[text], [text, image], [text, video], [text, audio]]) {
    assert.doesNotThrow(() =>
      validateOpenAPIExamples(
        responseExample('ModelArkVideoCreateRequest', {
          model: 'customer-video-model',
          content,
        }),
        approved
      )
    )
  }
  for (const item of [
    { type: image.type, image_url: image.image_url },
    { type: video.type, video_url: video.video_url },
    { type: audio.type, audio_url: audio.audio_url },
    { ...image, role: 'reference_audio' },
    { ...video, role: 'first_frame' },
    { ...audio, role: 'reference_video' },
    { ...image, video_url: video.video_url },
    { ...text, image_url: image.image_url },
    { type: 'image_url', role: 'first_frame' },
    { type: 'text' },
  ]) {
    assert.throws(
      () =>
        validateOpenAPIExamples(
          responseExample('ModelArkVideoCreateRequest', {
            model: 'customer-video-model',
            content: [item],
          }),
          approved
        ),
      /示例不符合合同/
    )
  }
})

test('hosted asset metadata and file / array parameter types are valid public examples', () => {
  const assets = {
    supported: true,
    documentation_path: '/docs/api-reference/assets',
    management_mode: 'platform_hosted',
    requires_model: true,
    reference_format: 'asset://<opaque-id>',
    media: [
      {
        kind: 'general',
        media_type: 'image',
        asset_group_requirement: 'optional',
      },
    ],
    operations: [],
  }
  assert.doesNotThrow(() =>
    validateOpenAPIExamples(responseExample('PublicAssetAPI', assets), approved)
  )
  for (const parameter of [
    { name: 'image', type: 'file', required: true },
    {
      name: 'images',
      type: 'array',
      item_type: 'string',
      required: false,
      min_items: 1,
      max_items: 14,
    },
  ]) {
    assert.doesNotThrow(() =>
      validateOpenAPIExamples(
        responseExample('PublicAPIParameter', parameter),
        approved
      )
    )
  }
})

test('missing local schema references fail validation', () => {
  const document = responseExample('ImageTaskStatus', {
    id: 'task-example',
    object: 'image_task',
    status: 'queued',
  })
  delete document.components.schemas.ImageTaskError
  assert.throws(
    () => validateOpenAPIExamples(document, approved),
    /reference|引用/
  )
})

test('unpublished operations do not expand the example validation scope', () => {
  const document = responseExample('DoesNotExist', {})
  assert.equal(validateOpenAPIExamples(document, new Set()), 0)
})

test('Markdown JSON and inline curl bodies are parsed without executing commands', () => {
  const page = { file: 'example.md' }
  const body =
    '```json\n{"ok":true}\n```\n```bash\ncurl --data-raw \'{"n":1}\' example.invalid\n```'
  assert.equal(validateMarkdownJSONExamples([{ page, body }]), 2)
  for (const invalid of [
    '```json\n{"ok":}\n```',
    '```bash\ncurl -d \'{"n":}\' example.invalid\n```',
  ]) {
    assert.throws(
      () => validateMarkdownJSONExamples([{ page, body: invalid }]),
      /Markdown JSON 示例无效/
    )
  }
})

test('image response formats distinguish URL results from image_url reference inputs', () => {
  for (const [schema, input] of [
    [
      'ImageGenerationRequest',
      { model: 'customer-image-model', prompt: 'Red cup' },
    ],
    [
      'NativeImageEditJSONRequest',
      {
        model: 'customer-image-model',
        prompt: 'Red cup',
        images: [{ image_url: 'https://example.com/reference.png' }],
      },
    ],
    [
      'UnifiedImageEditJSONRequest',
      {
        model: 'customer-image-model',
        prompt: 'Red cup',
        image: 'https://example.com/reference.png',
      },
    ],
    [
      'ImageEditRequest',
      { model: 'customer-image-model', prompt: 'Red cup', image: 'file bytes' },
    ],
  ]) {
    for (const format of ['url', 'b64_json']) {
      assert.doesNotThrow(() =>
        validateOpenAPIExamples(
          responseExample(schema, { ...input, response_format: format }),
          approved
        )
      )
    }
    assert.throws(
      () =>
        validateOpenAPIExamples(
          responseExample(schema, { ...input, response_format: 'image_url' }),
          approved
        ),
      /示例不符合合同/
    )
  }
})

test('published image result examples contain usable output fields and match response schemas', () => {
  for (const name of ['generations', 'edits', 'tasks']) {
    const page = readFileSync(
      new URL(
        `../../public/docs-content/zh/api-reference/images/${name}.md`,
        import.meta.url
      ),
      'utf8'
    )
    let results = 0
    for (const [, body] of page.matchAll(/^```json\n([\s\S]*?)^```/gm)) {
      const value = JSON.parse(body)
      if (!Array.isArray(value.data)) continue
      let schema = 'ImageResponse'
      if (value.error?.code === 'image_delivery_failed') {
        schema = 'ImageDeliveryError'
      } else if (value.object === 'image_task') {
        schema = 'ImageTaskStatus'
      }
      assert.doesNotThrow(() =>
        validateOpenAPIExamples(responseExample(schema, value), approved)
      )
      for (const item of value.data) {
        assert.equal(
          Object.hasOwn(item, 'image_url'),
          false,
          'image_url is an input reference, not an image result'
        )
        assert.ok(
          item.url || item.b64_json,
          'download examples need an actual result field'
        )
        if (item.url) assert.equal(new URL(item.url).protocol, 'https:')
        if (item.b64_json) {
          const bytes = Buffer.from(item.b64_json, 'base64')
          assert.equal(
            bytes.toString('base64'),
            item.b64_json,
            'use complete Base64 rather than an ellipsis or Data URL'
          )
          assert.equal(
            bytes.subarray(0, 8).toString('hex'),
            '89504e470d0a1a0a',
            'the example promises a PNG'
          )
        }
      }
      results += 1
    }
    assert.ok(results > 0, `${name} must demonstrate image output`)
  }
})

test('image parameter schemas validate published dimensions and per-image decoded byte limits', () => {
  const valid = {
    name: 'size',
    type: 'string',
    required: false,
    default_value: 'auto',
    size_constraints: {
      format: 'WxH',
      min_dimension: 1,
      max_dimension: 4096,
      aspect_ratios: ['1:1', '16:9'],
    },
  }
  assert.doesNotThrow(() =>
    validateOpenAPIExamples(
      responseExample('PublicAPIParameter', valid),
      approved
    )
  )
  assert.doesNotThrow(() =>
    validateOpenAPIExamples(
      responseExample('PublicAPIParameter', {
        name: 'images',
        type: 'array',
        required: false,
        item_type: 'string',
        max_decoded_bytes: 7000000,
      }),
      approved
    )
  )
  for (const value of [
    {
      ...valid,
      size_constraints: { ...valid.size_constraints, max_dimension: 0 },
    },
    {
      ...valid,
      size_constraints: { ...valid.size_constraints, format: '16:9' },
    },
    { name: 'image', type: 'string', required: false, max_decoded_bytes: -1 },
  ]) {
    assert.throws(
      () =>
        validateOpenAPIExamples(
          responseExample('PublicAPIParameter', value),
          approved
        ),
      /示例不符合合同/
    )
  }
})
