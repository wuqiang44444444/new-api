// Validate JSON, then change whitespace only. Parsing and reserializing would
// round large numeric identifiers, collapse duplicate keys and change escapes.
export function formatEvidenceJSON(raw: string): string {
  JSON.parse(raw)
  let formatted = ''
  let quoted = false
  let escaped = false
  let depth = 0
  let previous = ''
  const newline = () => `\n${'  '.repeat(Math.min(depth, 20))}`
  for (let i = 0; i < raw.length; i++) {
    const char = raw[i]
    if (quoted) {
      formatted += char
      if (escaped) escaped = false
      else if (char === '\\') escaped = true
      else if (char === '"') quoted = false
      continue
    }
    if (/\s/.test(char)) continue
    if (char === '"') {
      quoted = true
      formatted += char
    } else if (char === '{' || char === '[') {
      formatted += char
      depth++
      let nextIndex = i + 1
      while (nextIndex < raw.length && /\s/.test(raw[nextIndex])) nextIndex++
      const next = raw[nextIndex]
      if (next !== '}' && next !== ']') formatted += newline()
    } else if (char === '}' || char === ']') {
      depth--
      if (previous !== '{' && previous !== '[') formatted += newline()
      formatted += char
    } else if (char === ',') formatted += char + newline()
    else if (char === ':') formatted += ': '
    else formatted += char
    previous = char
  }
  return formatted
}
