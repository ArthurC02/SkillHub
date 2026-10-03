export type GuardedAction = string | null

declare module 'claude-code' {
  interface PluginState {
    'skillhub-guards': {
      touched: string[]
      lastDenied: GuardedAction
      allowance: GuardedAction
    }
  }
}
