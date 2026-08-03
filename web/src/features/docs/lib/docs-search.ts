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
import type { DocsSearchEntry } from '../types'

export function searchDocsEntries(entries: DocsSearchEntry[], query: string) {
  const terms = query
    .normalize('NFKC')
    .toLocaleLowerCase()
    .split(/\s+/)
    .filter(Boolean)
  if (terms.length === 0) return entries.slice(0, 8)

  return entries
    .map((entry) => {
      const title = entry.title.toLocaleLowerCase()
      const metadata = [
        entry.description,
        ...entry.keywords,
        ...entry.headings.map((heading) => heading.text),
      ]
        .join(' ')
        .toLocaleLowerCase()
      const score = terms.reduce((total, term) => {
        if (title === term) return total + 20
        if (title.includes(term)) return total + 10
        if (metadata.includes(term)) return total + 3
        return total
      }, 0)
      return { entry, score }
    })
    .filter((result) => result.score > 0)
    .sort(
      (left, right) =>
        right.score - left.score ||
        left.entry.title.localeCompare(right.entry.title, 'zh')
    )
    .slice(0, 8)
    .map((result) => result.entry)
}
