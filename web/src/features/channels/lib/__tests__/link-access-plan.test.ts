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
import { describe, expect, test } from 'vitest'

import type { LinkImplementation } from '../../types'
import {
  deriveLinkPublicationPreviews,
  linkAccessPlanAutofill,
  linkAccessPlanLabel,
  linkAccessPlanProviderModelDefaults,
  linkAccessPlansForChannelType,
} from '../link-access-plan'

const implementation = {
  id: 'example',
  version: 'v1',
  content_hash: 'sha256:test',
  provider: 'Provider',
  plan_name: 'Video Generation',
  contract_id: 'contract',
  public_skus: ['link-sku'],
  channel_type: 54,
  required_video_profile: 'third_party_relay',
  required_create_path: '/v1/media/generations',
  required_query_path: '/v1/media/tasks/{task_id}',
  execution_bindings: [
    {
      route_family: 'modelark_video',
      action: 'create',
      profile: 'third_party_relay',
      provider_model: 'provider-model',
      link_sku: 'link-sku',
    },
    {
      route_family: 'kling_video',
      action: 'create',
      profile: 'official',
      provider_model: 'provider-model',
      link_sku: 'link-sku',
    },
  ],
  asset_capability: {
    supports_managed_assets: false,
    supports_mixed_media_paths: false,
  },
  task_contract: 'task',
  billing_contract: 'billing',
} satisfies LinkImplementation

describe('Link access plan projection', () => {
  test('uses the registered Link plan name instead of its implementation ID', () => {
    expect(linkAccessPlanLabel(implementation)).toBe(
      'Provider · Video Generation'
    )
  })

  test('lists plans by channel type without filtering on customer model names', () => {
    expect(
      linkAccessPlansForChannelType(
        [implementation, { ...implementation, id: 'other', channel_type: 50 }],
        54
      ).map((plan) => plan.id)
    ).toEqual(['example'])
  })

  test('defaults video models from the selected create profile without using Link SKUs', () => {
    expect(linkAccessPlanProviderModelDefaults(implementation)).toEqual([
      'provider-model',
    ])
  })

  test('does not default provider models across multiple video route families', () => {
    expect(
      linkAccessPlanProviderModelDefaults({
        ...implementation,
        execution_bindings: [
          ...implementation.execution_bindings,
          {
            route_family: 'another_video_family',
            action: 'create',
            profile: 'third_party_relay',
            provider_model: 'another-provider-model',
            link_sku: 'link-sku',
          },
        ],
      })
    ).toEqual([])
  })

  test('does not project image implementation models into the video defaults', () => {
    expect(
      linkAccessPlanProviderModelDefaults({
        ...implementation,
        required_video_profile: undefined,
        execution_bindings: [
          {
            route_family: 'image_generation',
            action: 'create',
            profile: 'media_task_image_blocking',
            provider_model: 'provider-image-model',
            link_sku: 'image-link-sku',
          },
        ],
      })
    ).toEqual([])
  })

  test('derives a custom customer model through the ordinary mapping chain', () => {
    expect(
      deriveLinkPublicationPreviews(
        implementation,
        'customer-model',
        '{"customer-model":"provider-model"}'
      )
    ).toEqual([
      {
        customerModel: 'customer-model',
        providerModel: 'provider-model',
        linkSKU: 'link-sku',
        routeFamily: 'modelark_video',
      },
    ])
  })

  test('preserves mapping literals when matching execution bindings', () => {
    expect(
      deriveLinkPublicationPreviews(
        implementation,
        'customer-model',
        '{"customer-model":" provider-model "}'
      )
    ).toEqual([
      {
        customerModel: 'customer-model',
        providerModel: ' provider-model ',
        error: 'Provider model must match exactly one execution binding.',
      },
    ])
  })

  test('rejects non-object mapping JSON without crashing the projection', () => {
    expect(
      deriveLinkPublicationPreviews(
        implementation,
        'customer-model,customer-model',
        'null'
      )
    ).toEqual([
      {
        customerModel: 'customer-model',
        providerModel: '',
        error: 'Model mapping is not valid JSON.',
      },
    ])
  })

  test('autofill is projected from the selected plan', () => {
    const previews = deriveLinkPublicationPreviews(
      implementation,
      'customer-model',
      '{"customer-model":"provider-model"}'
    )
    expect(linkAccessPlanAutofill(implementation, previews)).toEqual({
      video_upstream_profile: 'third_party_relay',
      asset_upstream_profile: 'none',
      video_upstream_create_path: '/v1/media/generations',
      video_upstream_query_path_template: '/v1/media/tasks/{task_id}',
      asset_min_url_ttl_seconds: 0,
      advanced_custom: '',
    })
  })

  test('defaults a required asset profile fetch window to one hour', () => {
    expect(
      linkAccessPlanAutofill(
        {
          ...implementation,
          required_asset_profile: 'relay_assets',
        },
        []
      ).asset_min_url_ttl_seconds
    ).toBe(3600)
  })

  test('derives route family from the unique registered execution binding', () => {
    const renamedRouteImplementation = {
      ...implementation,
      execution_bindings: [
        {
          route_family: 'future_video_family',
          action: 'create',
          profile: 'third_party_relay',
          provider_model: 'provider-model',
          link_sku: 'link-sku',
        },
      ],
    }

    const previews = deriveLinkPublicationPreviews(
      renamedRouteImplementation,
      'customer-model',
      '{"customer-model":"provider-model"}'
    )

    expect(previews[0]?.routeFamily).toBe('future_video_family')
  })

  test('recomputes SKU-specific create path and clears stale plan values', () => {
    const skuPathImplementation = {
      ...implementation,
      required_create_path: undefined,
      required_query_path: undefined,
      required_sku_create_paths: [
        { public_sku: 'link-sku', create_path: '/v1/first' },
      ],
    }

    const beforeModels = linkAccessPlanAutofill(skuPathImplementation, [])
    const previews = deriveLinkPublicationPreviews(
      skuPathImplementation,
      'customer-model',
      '{"customer-model":"provider-model"}'
    )
    const afterModels = linkAccessPlanAutofill(skuPathImplementation, previews)

    expect(beforeModels.video_upstream_create_path).toBe('')
    expect(afterModels.video_upstream_create_path).toBe('/v1/first')
    expect(afterModels.video_upstream_query_path_template).toBe('')
    expect(afterModels.advanced_custom).toBe('')
  })

  test('matches registered Feicai, Moxing image, and Kling execution bindings', () => {
    const cases: Array<{
      implementation: LinkImplementation
      customerModel: string
      mapping: string
      linkSKU: string
      routeFamily: string
    }> = [
      {
        implementation: {
          ...implementation,
          id: 'feicai.seedance-videos',
          provider: '飞彩',
          public_skus: ['seedance-2.0-standard-720p'],
          required_video_profile: 'third_party_json_video_media_arrays',
          execution_bindings: [
            {
              route_family: 'modelark_video',
              action: 'create',
              profile: 'third_party_json_video_media_arrays',
              provider_model: 'seedance-2.0-vip-720p-azhw-feicai',
              link_sku: 'seedance-2.0-standard-720p',
            },
          ],
        },
        customerModel: 'customer-feicai',
        mapping: '{"customer-feicai":"seedance-2.0-vip-720p-azhw-feicai"}',
        linkSKU: 'seedance-2.0-standard-720p',
        routeFamily: 'modelark_video',
      },
      {
        implementation: {
          ...implementation,
          id: 'moxing.images.media-task',
          provider: 'Moxing',
          channel_type: 58,
          public_skus: ['nano-banana-2'],
          required_video_profile: undefined,
          required_create_path: undefined,
          required_query_path: undefined,
          required_routes: [
            {
              public_sku: 'nano-banana-2',
              incoming_path: '/v1/images/generations',
              upstream_path: '/v1/media/generations',
              converter: 'media_task_image_blocking',
              auth_type: 'header',
            },
          ],
          execution_bindings: [
            {
              route_family: 'image_generation',
              action: 'create',
              profile: 'media_task_image_blocking',
              provider_model: 'gemini-3.1-flash-image-preview-usage',
              link_sku: 'nano-banana-2',
            },
          ],
        },
        customerModel: 'customer-image',
        mapping: '{"customer-image":"gemini-3.1-flash-image-preview-usage"}',
        linkSKU: 'nano-banana-2',
        routeFamily: 'image_generation',
      },
      {
        implementation: {
          ...implementation,
          id: 'kling.videos-official',
          provider: 'Kling',
          channel_type: 50,
          public_skus: ['kling-v1'],
          required_video_profile: 'official',
          required_create_path: undefined,
          required_query_path: undefined,
          execution_bindings: [
            {
              route_family: 'kling_video',
              action: 'create',
              profile: 'official',
              provider_model: 'kling-v1',
              link_sku: 'kling-v1',
            },
          ],
        },
        customerModel: 'customer-kling',
        mapping: '{"customer-kling":"kling-v1"}',
        linkSKU: 'kling-v1',
        routeFamily: 'kling_video',
      },
    ]

    for (const testCase of cases) {
      const previews = deriveLinkPublicationPreviews(
        testCase.implementation,
        testCase.customerModel,
        testCase.mapping
      )
      expect(previews[0]?.linkSKU).toBe(testCase.linkSKU)
      expect(previews[0]?.routeFamily).toBe(testCase.routeFamily)
    }

    const moxingProjection = linkAccessPlanAutofill(
      cases[1].implementation,
      deriveLinkPublicationPreviews(
        cases[1].implementation,
        cases[1].customerModel,
        cases[1].mapping
      )
    )
    expect(moxingProjection.advanced_custom).toMatch(
      /nano-banana-2|customer-image/
    )
  })
})
