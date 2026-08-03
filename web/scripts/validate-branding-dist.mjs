import { readFile } from 'node:fs/promises'
import path from 'node:path'
import { fileURLToPath } from 'node:url'

const projectRoot = path.resolve(
  path.dirname(fileURLToPath(import.meta.url)),
  '..'
)
const html = await readFile(path.join(projectRoot, 'dist/index.html'), 'utf8')
const iconLinks = html.match(
  /<link\b[^>]*\brel=["'][^"']*icon[^"']*["'][^>]*>/gi
)

if (!iconLinks?.length) {
  throw new Error('Branding validation failed: no favicon links found')
}

const renderedIconLinks = iconLinks.join('\n')
const requiredAssets = [
  '/tokenai-favicon.svg',
  '/tokenai-logo.png',
  '/tokenai-favicon.ico',
]

for (const asset of requiredAssets) {
  if (!renderedIconLinks.includes(`href="${asset}"`)) {
    throw new Error(`Branding validation failed: missing ${asset}`)
  }
}

if (renderedIconLinks.includes('href="/favicon.ico"')) {
  throw new Error(
    'Branding validation failed: upstream /favicon.ico overrides TokenAI favicon'
  )
}

console.log('Branding dist validation passed.')
