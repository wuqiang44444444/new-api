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
export function parseBillingPreviewContext(
  time: string,
  json: string
): {
  pricingTime?: string
  body?: Record<string, unknown>
  headers?: Record<string, string>
  error?: string
} {
  const date = time ? new Date(time) : undefined
  if (date && !Number.isFinite(date.getTime())) {
    return { error: 'Enter a valid simulation time.' }
  }
  try {
    const value = json.trim() ? JSON.parse(json) : {}
    if (!value || typeof value !== 'object' || Array.isArray(value)) {
      throw new Error()
    }
    if (Object.keys(value).some((key) => key !== 'body' && key !== 'headers')) {
      throw new Error()
    }
    if (
      value.body !== undefined &&
      (!value.body ||
        typeof value.body !== 'object' ||
        Array.isArray(value.body))
    ) {
      throw new Error()
    }
    if (
      value.headers !== undefined &&
      (!value.headers ||
        typeof value.headers !== 'object' ||
        Array.isArray(value.headers) ||
        Object.values(value.headers).some((item) => typeof item !== 'string'))
    ) {
      throw new Error()
    }
    return {
      pricingTime: date?.toISOString(),
      body: value.body,
      headers: value.headers,
    }
  } catch {
    return { error: 'Use a JSON object with body and string-valued headers.' }
  }
}
