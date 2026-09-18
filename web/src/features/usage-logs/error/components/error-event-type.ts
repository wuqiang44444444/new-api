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
// 事件类型的展示标签；未知/空类型按 api_error 归类展示。
export function errorEventTypeLabel(
  eventType: string | undefined,
  t: (key: string) => string
): string {
  switch (eventType) {
    case 'channel_test':
      return t('Channel Test')
    case 'stream_error':
      return t('Stream Error')
    case 'task_failure':
      return t('Task Failure')
    default:
      return t('API Error')
  }
}
