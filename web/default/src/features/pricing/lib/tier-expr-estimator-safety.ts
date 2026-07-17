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

// The estimator executes only this small expression subset. Validate every
// token before handing the expression to JavaScript so an administrator-saved
// billing expression cannot access browser globals or use general JS syntax.
const ALLOWED_IDENTIFIERS = new Set([
  'p',
  'c',
  'len',
  'cr',
  'cc',
  'cc1h',
  'img',
  'img_o',
  'ai',
  'ao',
  'tier',
  'param',
  'header',
  'has',
  'max',
  'min',
  'abs',
  'ceil',
  'floor',
  'true',
  'false',
  'nil',
])

const TWO_CHAR_OPERATORS = new Set(['&&', '||', '==', '!=', '<=', '>='])
const ONE_CHAR_TOKENS = new Set([
  '(',
  ')',
  ',',
  '?',
  ':',
  '+',
  '-',
  '*',
  '/',
  '%',
  '<',
  '>',
  '!',
])
const MAX_ESTIMATOR_EXPRESSION_LENGTH = 20_000

export function prepareEstimatorExpression(expression: string): string {
  const body = expression
    .trim()
    .replace(/^v\d+:/, '')
    .trim()
  if (body.length > MAX_ESTIMATOR_EXPRESSION_LENGTH) {
    throw new Error('Expression is too long for browser preview')
  }

  let index = 0
  while (index < body.length) {
    const char = body[index]
    if (/\s/.test(char)) {
      index += 1
      continue
    }

    if (char === '"' || char === "'") {
      const quote = char
      index += 1
      let closed = false
      while (index < body.length) {
        const current = body[index]
        if (current === '\\') {
          index += 2
          continue
        }
        if (current === quote) {
          index += 1
          closed = true
          break
        }
        if (current === '\n' || current === '\r') {
          throw new Error('String literals cannot contain line breaks')
        }
        index += 1
      }
      if (!closed) throw new Error('Unterminated string literal')
      continue
    }

    const remaining = body.slice(index)
    const number = remaining.match(/^(?:\d+(?:\.\d*)?|\.\d+)(?:[eE][+-]?\d+)?/)
    if (number) {
      index += number[0].length
      continue
    }

    const identifier = remaining.match(/^[A-Za-z_][A-Za-z0-9_]*/)
    if (identifier) {
      if (!ALLOWED_IDENTIFIERS.has(identifier[0])) {
        throw new Error(
          `Unsupported identifier in browser preview: ${identifier[0]}`
        )
      }
      index += identifier[0].length
      continue
    }

    const twoChars = body.slice(index, index + 2)
    if (TWO_CHAR_OPERATORS.has(twoChars)) {
      index += 2
      continue
    }
    if (
      twoChars === '//' ||
      twoChars === '/*' ||
      twoChars === '**' ||
      twoChars === '=>'
    ) {
      throw new Error('Unsupported JavaScript syntax in browser preview')
    }
    if (ONE_CHAR_TOKENS.has(char)) {
      index += 1
      continue
    }

    throw new Error(
      `Unsupported character in browser preview expression: ${char}`
    )
  }

  return body
}
