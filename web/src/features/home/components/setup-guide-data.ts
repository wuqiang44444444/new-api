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
import { PanelsTopLeft, PanelLeft, Terminal } from 'lucide-react'

export type SetupSceneId = 'editor' | 'cli' | 'app'
export type SetupProtocol = 'openai' | 'anthropic'

export interface SetupTool {
  id: string
  scene: SetupSceneId
  name: string
  protocol: SetupProtocol
  summary: string
  location: string
  action: string
  verification: string
}

export const SETUP_SCENES = [
  { id: 'editor', label: 'Code editors', icon: PanelLeft },
  { id: 'cli', label: 'Command-line tools', icon: Terminal },
  { id: 'app', label: 'Desktop and web apps', icon: PanelsTopLeft },
] as const

export const SETUP_TOOLS: SetupTool[] = [
  {
    id: 'claude-code',
    scene: 'editor',
    name: 'Claude Code',
    protocol: 'anthropic',
    summary: 'Connect through Anthropic-compatible environment variables.',
    location: 'Terminal environment variables',
    action: 'Run claude to start',
    verification: 'Send a minimal request and review usage logs',
  },
  {
    id: 'curl',
    scene: 'cli',
    name: 'curl',
    protocol: 'openai',
    summary: 'Verify connectivity with one minimal HTTP request.',
    location: 'Terminal',
    action: 'Run the request command',
    verification: 'Confirm the response and usage log',
  },
  {
    id: 'cherry-studio',
    scene: 'app',
    name: 'Cherry Studio',
    protocol: 'openai',
    summary: 'Add an OpenAI-compatible provider in the model service settings.',
    location: 'Model service settings',
    action: 'Save and enable the provider',
    verification: 'Start a chat and review usage logs',
  },
]

export function buildSetupCode(tool: SetupTool, baseUrl: string): string {
  if (tool.id === 'claude-code') {
    return `export ANTHROPIC_BASE_URL="${baseUrl}"
export ANTHROPIC_API_KEY="sk-your-key"

claude`
  }

  if (tool.id === 'curl') {
    return `curl "${baseUrl}/v1/chat/completions" \\
  -H "Authorization: Bearer sk-your-key" \\
  -H "Content-Type: application/json" \\
  -d '{"model":"<available-model-id>","messages":[{"role":"user","content":"Hello"}]}'`
  }

  return `Protocol: OpenAI Compatible
Base URL: ${baseUrl}/v1
API Key: sk-your-key
Model: <available-model-id>`
}
