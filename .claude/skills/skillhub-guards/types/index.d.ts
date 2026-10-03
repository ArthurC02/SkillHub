export type GuardedAction = string | null
export type CIWatch = { sha: string; since: number } | null

declare module 'claude-code' {
  interface PluginState {
    'skillhub-guards': {
      touched: string[]
      lastDenied: GuardedAction
      allowance: GuardedAction
      ciWatch: CIWatch
    }
  }
}
