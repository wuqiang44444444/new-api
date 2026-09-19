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
// 自动检查分项事实的 detail 键；这些键只由后台自动检查事件写入，手工事件
// 不携带，展示层据此仅对自动事件渲染分项结果。
export const AUTO_CHECK_DETAIL_KEYS = [
  'connection_result',
  'probe_media',
  'check_result',
  'check_reason',
  'check_code',
  'upstream_status',
  'check_scope',
  'config_check',
  'generation_evidence',
  'upstream_request',
  'config_reason',
  'config_entry',
  'config_summary',
  'readonly_check',
  'billing_model',
] as const
