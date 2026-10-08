import type { On } from 'claude-code'
import { expect, test } from 'claude-code/testing'
import { repoRelative, shipPathProblem } from '../hooks/assist'

const ROOT = 'c:/repo'

test('a path under the project root becomes repo-relative on either slash and any case', () => {
  expect(repoRelative('C:\\Repo', 'c:/repo/apps/web/a.ts')).toBe('apps/web/a.ts')
  expect(repoRelative('c:/repo/', 'C:\\Repo\\docs\\x.md')).toBe('docs/x.md')
  expect(repoRelative('c:/repo', 'apps/web/a.ts')).toBe('apps/web/a.ts')
})

test('each named file is accepted', () => {
  expect(shipPathProblem(['apps/web/a.ts', 'docs/development/automation.md'])).toBeUndefined()
})

test('no files, the whole tree, a flag, a glob, a folder or a parent path is refused', () => {
  expect(shipPathProblem([])).toBe('name the files to commit')
  for (const loose of ['.', '-A', 'apps/*.ts', 'apps/web/', '../outside.ts']) {
    expect(shipPathProblem(['apps/web/a.ts', loose])).toBe(`"${loose}" is not one explicit file; name each file`)
  }
})

type Answers = {
  lint?: number; staged?: string; ancestor?: number; imageDiff?: string; push?: number; kind?: 'file' | 'dir'
}

const repo = (on: On, answers: Answers, runs: string[][], stdins: (string | undefined)[]) => {
  on('session.root', () => ({ value: ROOT }))
  on('fs.stat', () => ({ value: { kind: answers.kind ?? 'file', size: 1, mtimeMs: 0, isLink: false } }))
  on('process.run', (_, e) => {
    const argv = [...e.argv]
    runs.push(argv)
    stdins.push(e.init?.stdin)
    const verb = argv[0] === 'go' ? 'comment-lint' : argv[3] ?? ''
    const exitCode = verb === 'comment-lint' ? answers.lint ?? 0
      : verb === 'merge-base' ? answers.ancestor ?? 0
      : verb === 'push' ? answers.push ?? 0
      : 0
    const stdout = verb === 'diff' && argv.includes('--cached') ? answers.staged ?? ''
      : verb === 'diff' ? answers.imageDiff ?? ''
      : verb === 'rev-parse' ? 'abcdef0123456789\n'
      : ''
    return { value: { exitCode, stdout, stderr: '', isStdoutTruncated: false, isStderrTruncated: false } }
  })
}

const ship = { tool: 'mcp__skillhub-guards__ship' as const,
  paths: ['c:/repo/apps/web/a.ts', 'docs/x.md'], message: 'feat: x\n\nCo-Authored-By: C' }

const gitVerbs = (runs: string[][]) => runs.filter(argv => argv[0] === 'git').map(argv => argv.slice(3).join(' '))

test('a clean ship lints, stages each file, commits from stdin, checks fast-forward, pushes and watches CI', async ($, on) => {
  const runs: string[][] = []
  const stdins: (string | undefined)[] = []
  repo(on, {}, runs, stdins)
  const result = await $.tool.call(ship as never) as { result: string; isError?: boolean }
  expect(result.isError).toBeUndefined()
  expect(result.result).toMatch(/^pushed abcdef01; CI is being watched/)
  expect(runs[0]).toEqual(['go', '-C', 'c:/repo/tools/devctl', 'run', '.', 'comment-lint', 'apps/web/a.ts', 'docs/x.md'])
  expect(gitVerbs(runs)).toEqual([
    'add -- apps/web/a.ts docs/x.md',
    'commit -F - -- apps/web/a.ts docs/x.md',
    'fetch --quiet',
    'merge-base --is-ancestor origin/main HEAD',
    'rev-parse HEAD',
    'rev-parse --verify --quiet @{u}',
    'diff --name-only @{u}..HEAD -- infra/images/runtime-agent-sdk',
    'push --quiet',
    'rev-parse HEAD',
  ])
  expect(stdins[gitVerbs(runs).indexOf('commit -F - -- apps/web/a.ts docs/x.md') + 1]).toBe('feat: x\n\nCo-Authored-By: C')
})

test('a named domain-memory file makes the commit signed', async ($, on) => {
  const runs: string[][] = []
  repo(on, {}, runs, [])
  const result = await $.tool.call({ ...ship, paths: ['docs/domain-memory/registry/x.json'] } as never) as { result: string }
  expect(gitVerbs(runs)).toContain('commit -S -F - -- docs/domain-memory/registry/x.json')
  expect(result.result).toMatch(/\(signed\)/)
})

test('a domain-memory file another session staged neither signs nor joins the commit', async ($, on) => {
  const runs: string[][] = []
  repo(on, { staged: 'docs/domain-memory/registry/theirs.json\n' }, runs, [])
  const result = await $.tool.call(ship as never) as { result: string }
  expect(gitVerbs(runs)).toContain('commit -F - -- apps/web/a.ts docs/x.md')
  expect(result.result).not.toMatch(/\(signed\)/)
})

test('a comment-lint failure stops before anything is staged', async ($, on) => {
  const runs: string[][] = []
  repo(on, { lint: 1 }, runs, [])
  const result = await $.tool.call(ship as never) as { result: string; isError?: boolean }
  expect(result.isError).toBe(true)
  expect(result.result).toMatch(/^comment-lint refused/)
  expect(gitVerbs(runs)).toEqual([])
})

test('a branch behind origin/main is committed but not pushed', async ($, on) => {
  const runs: string[][] = []
  repo(on, { ancestor: 1 }, runs, [])
  const result = await $.tool.call(ship as never) as { result: string; isError?: boolean }
  expect(result.isError).toBe(true)
  expect(result.result).toMatch(/^committed, not pushed/)
  expect(gitVerbs(runs).some(verb => verb.startsWith('push'))).toBe(false)
})

test('an unallowed runtime image change is refused before the push', async ($, on) => {
  const runs: string[][] = []
  repo(on, { imageDiff: 'infra/images/runtime-agent-sdk/Dockerfile\n' }, runs, [])
  const result = await $.tool.call(ship as never) as { deny?: string }
  expect(result.deny).toMatch(/runtime image tag/)
  expect(gitVerbs(runs).some(verb => verb.startsWith('push'))).toBe(false)
})

test('a subagent may not ship', async ($, on) => {
  const runs: string[][] = []
  repo(on, {}, runs, [])
  const result = await $.tool.call({ ...ship, agentId: 'agent-1' } as never) as { deny?: string }
  expect(result.deny).toMatch(/subagent may not commit or push/)
  expect(runs).toEqual([])
})

test('a folder is refused before lint or Git runs', async ($, on) => {
  const runs: string[][] = []
  repo(on, { kind: 'dir' }, runs, [])
  const result = await $.tool.call(ship as never) as { result: string; isError?: boolean }
  expect(result.result).toBe('"apps/web/a.ts" is a directory; name each file')
  expect(runs).toEqual([])
})
