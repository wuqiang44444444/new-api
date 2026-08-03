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
import { z } from 'zod'

import type { ChannelFormValues } from './channel-form'

// DoubaoVideo (type 54) 第三方视频上游协议的表单联合校验（方案 §5.2/§6）。
// 规则与后端 dto.ValidateVideoUpstreamURL 对齐：后端是最终边界，前端校验只提前暴露错误。
// 抽到独立文件，避免在共享热文件 channel-form.ts 中堆积供应商专用逻辑（最小入侵）。
export function refineVideoUpstreamProfile(
  data: ChannelFormValues,
  ctx: z.RefinementCtx
): void {
  if (data.type !== 54) return
  const isThirdParty =
    data.video_upstream_profile === 'third_party_relay' ||
    data.video_upstream_profile === 'third_party_reverse_proxy' ||
    data.video_upstream_profile === 'third_party_json_video_omni_reference' ||
    data.video_upstream_profile === 'third_party_funcloud_seedance_v2'
  if (!isThirdParty) return

  const addIssue = (path: string, message: string) =>
    ctx.addIssue({ code: z.ZodIssueCode.custom, path: [path], message })

  if (!data.base_url?.trim()) {
    addIssue('base_url', 'Base URL is required for third-party video upstream')
  } else if (
    data.video_upstream_profile === 'third_party_json_video_omni_reference' ||
    data.video_upstream_profile === 'third_party_funcloud_seedance_v2'
  ) {
    try {
      const parsed = new URL(data.base_url.trim())
      if (
        parsed.protocol !== 'https:' ||
        parsed.username ||
        parsed.password ||
        parsed.search ||
        parsed.hash
      ) {
        addIssue(
          'base_url',
          'Selected video profile requires an HTTPS Base URL without userinfo, query, or fragment'
        )
      }
    } catch {
      addIssue(
        'base_url',
        'Selected video profile requires an HTTPS Base URL without userinfo, query, or fragment'
      )
    }
  }

  const createPath = data.video_upstream_create_path?.trim() || ''
  if (!createPath) {
    addIssue(
      'video_upstream_create_path',
      'Create path suffix is required for third-party video upstream'
    )
  } else if (!createPath.startsWith('/') || createPath.startsWith('//')) {
    addIssue(
      'video_upstream_create_path',
      'Create path suffix must start with a single /'
    )
  } else if (createPath.includes('{task_id}')) {
    addIssue(
      'video_upstream_create_path',
      'Create path suffix must not contain {task_id}'
    )
  } else if (/[?#]/.test(createPath)) {
    addIssue(
      'video_upstream_create_path',
      'Create path suffix must not include query or fragment'
    )
  }

  const queryTemplate = data.video_upstream_query_path_template?.trim() || ''
  const taskIdMatches = queryTemplate.match(/\{task_id\}/g)
  if (!queryTemplate) {
    addIssue(
      'video_upstream_query_path_template',
      'Query path template is required for third-party video upstream'
    )
  } else if (!queryTemplate.startsWith('/') || queryTemplate.startsWith('//')) {
    addIssue(
      'video_upstream_query_path_template',
      'Query path template must start with a single /'
    )
  } else if (!taskIdMatches || taskIdMatches.length !== 1) {
    addIssue(
      'video_upstream_query_path_template',
      'Query path template must contain exactly one {task_id}'
    )
  } else if (/[?#]/.test(queryTemplate)) {
    addIssue(
      'video_upstream_query_path_template',
      'Query path template must not include query or fragment'
    )
  }

  if (data.video_upstream_profile === 'third_party_funcloud_seedance_v2') {
    if (
      createPath !== '/api/v2/open/aigc/seedance2-0' &&
      createPath !== '/api/v2/open/aigc/seedance2-0-fast'
    ) {
      addIssue(
        'video_upstream_create_path',
        'FunCloud create path must select the Standard or Fast endpoint'
      )
    }
    if (queryTemplate !== '/api/v2/open/aigc/{task_id}') {
      addIssue(
        'video_upstream_query_path_template',
        'FunCloud query path must use /api/v2/open/aigc/{task_id}'
      )
    }
  }
}
