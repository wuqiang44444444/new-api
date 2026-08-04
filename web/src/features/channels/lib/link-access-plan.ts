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
import type { LinkImplementation } from '../types'

export type LinkPublicationPreview = {
  customerModel: string
  providerModel: string
  linkSKU?: string
  routeFamily?: string
  error?: string
}

export type LinkAccessPlanProjection = {
  video_upstream_profile: string
  asset_upstream_profile: string
  video_upstream_create_path: string
  video_upstream_query_path_template: string
  asset_min_url_ttl_seconds: number
  advanced_custom: string
}

export const EMPTY_LINK_ACCESS_PLAN_PROJECTION: LinkAccessPlanProjection = {
  video_upstream_profile: 'official',
  asset_upstream_profile: 'none',
  video_upstream_create_path: '',
  video_upstream_query_path_template: '',
  asset_min_url_ttl_seconds: 0,
  advanced_custom: '',
}

export function linkAccessPlansForChannelType(
  implementations: LinkImplementation[],
  channelType: number
): LinkImplementation[] {
  return implementations.filter(
    (implementation) => implementation.channel_type === channelType
  )
}

export function deriveLinkPublicationPreviews(
  implementation: LinkImplementation,
  models: string,
  modelMapping: string
): LinkPublicationPreview[] {
  const customerModels = [
    ...new Set(
      models
        .split(',')
        .map((model) => model.trim())
        .filter(Boolean)
    ),
  ]
  let mapping: Record<string, string> = {}
  try {
    if (modelMapping.trim()) {
      const parsed: unknown = JSON.parse(modelMapping)
      if (
        typeof parsed !== 'object' ||
        parsed === null ||
        Array.isArray(parsed)
      ) {
        throw new TypeError('model mapping must be an object')
      }
      mapping = parsed as Record<string, string>
    }
  } catch {
    return customerModels.map((customerModel) => ({
      customerModel,
      providerModel: '',
      error: 'Model mapping is not valid JSON.',
    }))
  }

  return customerModels.map((customerModel) => {
    let providerModel = customerModel
    const visited = new Set([providerModel])
    while (typeof mapping[providerModel] === 'string') {
      const next = mapping[providerModel].trim()
      if (!next || next === providerModel) break
      if (visited.has(next)) {
        return {
          customerModel,
          providerModel,
          error: 'Model mapping contains a cycle.',
        }
      }
      visited.add(next)
      providerModel = next
    }
    const expectedProfile = implementation.required_video_profile || 'official'
    const bindings = implementation.execution_bindings.filter((binding) => {
      if (
        binding.action !== 'create' ||
        binding.provider_model !== providerModel
      ) {
        return false
      }
      if (!implementation.required_routes?.length) {
        return binding.profile === expectedProfile
      }
      return implementation.required_routes?.some(
        (route) =>
          route.public_sku === binding.link_sku &&
          route.converter === binding.profile
      )
    })
    if (bindings.length !== 1) {
      return {
        customerModel,
        providerModel,
        error: 'Provider model must match exactly one execution binding.',
      }
    }
    return {
      customerModel,
      providerModel,
      linkSKU: bindings[0].link_sku,
      routeFamily: bindings[0].route_family,
    }
  })
}

export function linkAccessPlanAutofill(
  implementation: LinkImplementation,
  previews: LinkPublicationPreview[]
): LinkAccessPlanProjection {
  const values: LinkAccessPlanProjection = {
    ...EMPTY_LINK_ACCESS_PLAN_PROJECTION,
  }
  if (implementation.required_video_profile) {
    values.video_upstream_profile = implementation.required_video_profile
  }
  if (implementation.required_asset_profile) {
    values.asset_upstream_profile = implementation.required_asset_profile
  }
  if (implementation.required_create_path) {
    values.video_upstream_create_path = implementation.required_create_path
  } else {
    const linkSKUs = new Set(
      previews.flatMap((preview) => preview.linkSKU || [])
    )
    const paths = (implementation.required_sku_create_paths || [])
      .filter((requirement) => linkSKUs.has(requirement.public_sku))
      .map((requirement) => requirement.create_path)
    if (new Set(paths).size === 1) values.video_upstream_create_path = paths[0]
  }
  if (implementation.required_query_path) {
    values.video_upstream_query_path_template =
      implementation.required_query_path
  }
  if (implementation.asset_capability.asset_source_min_ttl_seconds) {
    values.asset_min_url_ttl_seconds =
      implementation.asset_capability.asset_source_min_ttl_seconds
  }

  if (implementation.required_routes?.length) {
    const routes = implementation.required_routes.flatMap((requirement) => {
      const customerModels = previews
        .filter((preview) => preview.linkSKU === requirement.public_sku)
        .map((preview) => preview.customerModel)
      if (customerModels.length === 0) return []
      return [
        {
          incoming_path: requirement.incoming_path,
          upstream_path: requirement.upstream_path,
          converter: requirement.converter,
          models: customerModels,
          auth:
            requirement.auth_type === 'header'
              ? {
                  type: 'header',
                  name: 'Authorization',
                  value: 'Bearer {api_key}',
                }
              : { type: requirement.auth_type },
        },
      ]
    })
    if (routes.length > 0) {
      values.advanced_custom = JSON.stringify({ advanced_routes: routes })
    }
  }
  return values
}
