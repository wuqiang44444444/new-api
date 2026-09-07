import { parseTaskArtifactsResponse } from '../lib/task-artifacts'
import { fireEvent, render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { LegacyVideoResult } from '../components/task-artifacts'

vi.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}))
describe('Task video result', () => {
  it('keeps the media address and actions after playback fails and allows retry', () => {
    const url =
      'https://gateway.example/v1/tasks/task-1/artifacts/video/content?access=test'
    const { container } = render(<LegacyVideoResult contentUrl={url} />)
    const video = container.querySelector('video')
    expect(video).not.toBeNull()
    if (!video) throw new Error('Video preview missing')
    fireEvent.error(video)
    expect(screen.getByRole('alert')).toBeTruthy()
    expect(
      screen.getByRole('link', { name: 'Open' }).getAttribute('href')
    ).toBe(url)
    expect(screen.getByRole('button', { name: 'Copy link' })).toBeTruthy()
    expect(screen.getByText(url)).toBeTruthy()
    fireEvent.click(screen.getByRole('button', { name: 'Retry' }))
    expect(container.querySelector('video')?.getAttribute('src')).toBe(url)
  })
})

it('resolves dashboard media on the current site without using the public domain', () => {
  const path =
    `/v1/tasks/task-1/artifacts/video/content?access=${  'A'.repeat(43)}`
  const result = parseTaskArtifactsResponse({
    success: true,
    data: { artifacts: [], legacy_content_url: path },
  })
  expect(result.legacyContentUrl).toBe(window.location.origin + path)
  for (const bad of [
    `//foreign.example${  path}`,
    `/v1/tasks/\\foreign.example/content?access=${  'A'.repeat(43)}`,
  ]) {
    expect(() =>
      parseTaskArtifactsResponse({
        success: true,
        data: { artifacts: [], legacy_content_url: bad },
      })
    ).toThrow()
  }
})
