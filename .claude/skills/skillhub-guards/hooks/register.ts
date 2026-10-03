import type { EngineInterface, Register } from 'claude-code'

const ROLES = ['skillhub-writer', 'skillhub-verify', 'skillhub-mutation']
const FAMILIES = ['fable', 'opus', 'sonnet', 'haiku']
const RUNTIME_IMAGE = 'infra/images/runtime-agent-sdk'
const PAID_STACK = /\btask\s+dev:model\b/
const GIT_PUSH = /\bgit\b[^|;&\n]*\bpush\b/
const ALLOW_HINT = 'If the owner agrees, ask them to type /guard-allow, then retry once.'

const familyOf = (model: string | undefined) =>
  FAMILIES.find(family => (model ?? '').toLowerCase().includes(family))

const normalize = (path: string) => path.replace(/\\/g, '/').toLowerCase()

const directoryOf = (path: string) => {
  const slash = Math.max(path.lastIndexOf('/'), path.lastIndexOf('\\'))
  return slash > 0 ? path.slice(0, slash) : '.'
}

export const spawnProblem = (
  model: string | undefined, subagentType: string, parentModel: string,
): string | undefined => {
  if (model === undefined && !ROLES.includes(subagentType)) {
    return `name a model for this subagent or use one of ${ROLES.join(', ')}; ` +
      'an unnamed model inherits the dispatcher\'s flagship model (AGENTS.md 開發自動化 3)'
  }
  const family = familyOf(model)
  if (family === 'fable' || (family !== undefined && family === familyOf(parentModel))) {
    return `model ${model} is the dispatcher's own tier; subagents start from the lowest tier ` +
      'that can finish the task (AGENTS.md 開發自動化 3)'
  }
  return undefined
}

async function touched($: EngineInterface): Promise<string[]> {
  return (await $.state.get({ plugin: 'skillhub-guards', key: 'touched' })).value ?? []
}

async function consumeAllowance($: EngineInterface, action: string): Promise<boolean> {
  const allowance = (await $.state.get({ plugin: 'skillhub-guards', key: 'allowance' })).value
  if (allowance !== action) return false
  await $.state.set({ plugin: 'skillhub-guards', key: 'allowance' }, null)
  return true
}

async function refuse($: EngineInterface, action: string, reason: string) {
  await $.state.set({ plugin: 'skillhub-guards', key: 'lastDenied' }, action)
  return { deny: `skillhub-guards: ${reason} ${ALLOW_HINT}` }
}

export const register: Register = on => {
  on('session.start', async ($, e, next) => {
    await $.command.register({
      name: 'guard-allow',
      description: 'Let the action skillhub-guards last refused through, once',
    })
    return next(e)
  })

  on('command.run', { command: 'guard-allow' }, async ($, e) => {
    if (e.origin.kind !== 'composer' && e.origin.kind !== 'bridge') {
      return { text: 'skillhub-guards: only a person typing /guard-allow can grant it.' }
    }
    const lastDenied = (await $.state.get({ plugin: 'skillhub-guards', key: 'lastDenied' })).value
    if (!lastDenied) return { text: 'skillhub-guards: nothing has been refused yet.' }
    await $.state.set({ plugin: 'skillhub-guards', key: 'allowance' }, lastDenied)
    return { text: `skillhub-guards: the next attempt at ${lastDenied} goes through, once.` }
  })

  on('agent.spawn', ($, e, next) => {
    const problem = spawnProblem(e.model, e.subagentType, e.parentModel)
    return problem ? { deny: `skillhub-guards: ${problem}` } : next(e)
  })

  on('tool.call', { tool: 'Bash' }, async ($, e, next) => {
    if (e.agentId !== undefined && PAID_STACK.test(e.command)) {
      return { deny: 'skillhub-guards: a subagent may not start the paid model stack (AGENTS.md 開發自動化 2).' }
    }
    if (!GIT_PUSH.test(e.command)) return next(e)
    const head = (await $.process.run(['git', 'rev-parse', 'HEAD'])).stdout.trim()
    const upstream = await $.process.run(['git', 'rev-parse', '--verify', '--quiet', '@{u}'])
    const base = upstream.exitCode === 0 ? '@{u}' : 'origin/main'
    const changed = await $.process.run(['git', 'diff', '--name-only', `${base}..HEAD`, '--', RUNTIME_IMAGE])
    const action = `push of ${head.slice(0, 8)} touching ${RUNTIME_IMAGE}`
    if (changed.stdout.trim() === '' || await consumeAllowance($, action)) return next(e)
    return refuse($, action,
      `this push publishes a new runtime image tag, which cannot be withdrawn; ask the owner first ` +
      `(.claude/rules/tests.md).`)
  })

  on('tool.call', { tool: ['Edit', 'Write'] }, async ($, e, next) => {
    if (e.tool !== 'Edit' && e.tool !== 'Write') return next(e)
    const path = e.file_path
    const key = normalize(path)
    if (!(await touched($)).includes(key)) {
      const status = await $.process.run(['git', '-C', directoryOf(path), 'status', '--porcelain', '--', path])
      const action = `edit of ${path}`
      if (status.exitCode === 0 && status.stdout.trim() !== '' && !(await consumeAllowance($, action))) {
        return refuse($, action,
          `${path} has uncommitted changes this session did not make; keep them and report ` +
          `(AGENTS.md 開發自動化 3).`)
      }
    }
    const result = await next(e)
    if (result.deny === undefined && result.isError !== true) {
      const list = await touched($)
      if (!list.includes(key)) {
        await $.state.set({ plugin: 'skillhub-guards', key: 'touched' }, [...list, key])
      }
    }
    return result
  })
}
