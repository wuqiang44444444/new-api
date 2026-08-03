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
export type ResolvedDocsLink = {
  external: boolean
  href: string
}

function hasUnsafeLinkCharacter(value: string): boolean {
  return [...value].some((character) => {
    const code = character.codePointAt(0) ?? 0
    return code <= 31 || code === 127 || character === '\\'
  })
}

export function resolveDocsLink(value: unknown): ResolvedDocsLink {
  if (typeof value !== 'string') {
    return { external: false, href: '/docs' }
  }

  const candidate = value.trim()
  if (!candidate || hasUnsafeLinkCharacter(candidate)) {
    return { external: false, href: '/docs' }
  }

  if (candidate.startsWith('/') && !candidate.startsWith('//')) {
    return { external: false, href: candidate }
  }

  try {
    const url = new URL(candidate)
    if (
      (url.protocol === 'http:' || url.protocol === 'https:') &&
      url.username === '' &&
      url.password === ''
    ) {
      return { external: true, href: url.href }
    }
  } catch {
    // Invalid and relative values fall back to the built-in docs route.
  }

  return { external: false, href: '/docs' }
}
