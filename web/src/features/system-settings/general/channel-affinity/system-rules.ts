// The server rejects saves that delete these built-in rules.
export function isSystemAffinityRule(name: string) {
  return name === 'claude cli trace' || name === 'codex cli trace'
}
