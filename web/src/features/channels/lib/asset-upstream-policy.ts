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
export const DEFAULT_ASSET_MIN_URL_TTL_SECONDS = 3600

export function assetMinURLTTLWithDefault(
  profile: string | null | undefined,
  currentTTLSeconds: number | undefined
): number {
  const currentTTL = currentTTLSeconds || 0
  if (!profile || profile === 'none' || currentTTL > 0) {
    return currentTTL
  }
  return DEFAULT_ASSET_MIN_URL_TTL_SECONDS
}
