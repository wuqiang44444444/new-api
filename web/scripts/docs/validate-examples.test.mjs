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
