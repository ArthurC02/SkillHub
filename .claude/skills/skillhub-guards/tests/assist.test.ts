import type { On } from 'claude-code'
import { expect, test } from 'claude-code/testing'
import { ciMessage, ciVerdict, dsnFromEnvFile, mutationVerdict, replaceOnce, withTestDatabase } from '../hooks/assist'

const DSN = 'postgres://u:p@localhost:5432/x_test'
const ENV = `SKILLHUB_TEST_DATABASE_URL='${DSN}' SKILLHUB_REQUIRE_DB=1`

test('a platform go test gets the test database, the require guard and an uncached run', () => {
  expect(withTestDatabase('cd apps/platform && go test ./internal/x/', DSN))
    .toBe(`cd apps/platform && ${ENV} go test -count=1 ./internal/x/`)
})

test('go -C apps/platform test and an rtk prefix keep their shape', () => {
  expect(withTestDatabase('rtk proxy go -C apps/platform test -run X ./...', DSN))
    .toBe(`${ENV} rtk proxy go -C apps/platform test -count=1 -run X ./...`)
})

test('a run that already sets -count keeps it', () => {
  expect(withTestDatabase('go -C apps/platform test -count=3 ./...', DSN))
    .toBe(`${ENV} go -C apps/platform test -count=3 ./...`)
})

test('a command that names its own database is left alone', () => {
  expect(withTestDatabase(`SKILLHUB_TEST_DATABASE_URL=x go -C apps/platform test ./...`, DSN)).toBeUndefined()
})

test('go test outside apps/platform, or a platform command that is not go test, is left alone', () => {
  expect(withTestDatabase('go -C tools/devctl test ./...', DSN)).toBeUndefined()
  expect(withTestDatabase('go -C apps/platform vet ./...', DSN)).toBeUndefined()
  expect(withTestDatabase('grep -rn "go test" apps/platform/AGENTS.md', DSN)).toBeUndefined()
})

test('the DSN is read from .env, quoted or not, and an empty value counts as unset', () => {
  expect(dsnFromEnvFile(`A=1\nSKILLHUB_TEST_DATABASE_URL="${DSN}"\n`)).toBe(DSN)
  expect(dsnFromEnvFile(`SKILLHUB_TEST_DATABASE_URL=${DSN}\r\n`)).toBe(DSN)
  expect(dsnFromEnvFile('SKILLHUB_TEST_DATABASE_URL=\n')).toBeUndefined()
})

test('ci-status lines are read as green, red or pending, and anything else as unknown', () => {
  expect(ciVerdict('aff79dc (aff79dc4700b014fed6f69a8536a34254785e4cb): green\n  CI: completed/success')).toBe('green')
  expect(ciVerdict('2514793 (25147931a393fd8de08971be7d0a266893d11cb5): red\n    failure platform')).toBe('red')
  expect(ciVerdict('cd9cf7e (cd9cf7ec1270be9697dc9f95f2f0a1d8f55bf86b): pending')).toBe('pending')
  expect(ciVerdict('devctl: rate limited')).toBeUndefined()
})

test('a red CI message names each failing job, a green one only the commit', () => {
  const red = '2514793 (25147931a393fd8de08971be7d0a266893d11cb5): red\n  CI: completed/failure https://x\n' +
    '    failure platform: Platform tests\n    failure web: Web tests\n    did not run: llm\n'
  expect(ciMessage('25147931a393fd8de', 'red', red)).toBe('CI 25147931: red — failure platform: Platform tests; failure web: Web tests')
  expect(ciMessage('aff79dc4700b014f', 'green', 'aff79dc (aff79dc4700b014f): green')).toBe('CI aff79dc4: green')
})

test('a snippet that occurs exactly once is replaced', () => {
  expect(replaceOnce('a < b; c', '<', '<=')).toBe('a <= b; c')
})

test('a missing, empty or repeated snippet is refused', () => {
  expect(replaceOnce('a < b', '>', '>=')).toEqual({ problem: 'the text to break was not found in the file' })
  expect(replaceOnce('a < b', '', 'x')).toEqual({ problem: 'the text to break was not found in the file' })
  expect(replaceOnce('a < b < c', '<', '<=')).toEqual({
    problem: 'the text to break occurs more than once; give a longer snippet',
  })
})

test('exit 0 is GREEN, a failing test is RED, a compile error is BUILD FAILED', () => {
  expect(mutationVerdict(0, 'ok  pkg 0.1s').verdict).toMatch(/^GREEN/)
  const red = mutationVerdict(1, '=== RUN TestA\n--- FAIL: TestA (0.01s)\n    a_test.go:9: got 1\nFAIL\tpkg\t0.2s')
  expect(red.verdict).toMatch(/^RED/)
  expect(red.summary).toBe('--- FAIL: TestA (0.01s)\nFAIL\tpkg\t0.2s')
  expect(mutationVerdict(1, './a.go:3:2: undefined: x\nFAIL\tpkg [build failed]').verdict).toMatch(/^BUILD FAILED/)
})

const ROOT = 'c:/repo'

const key = (path: string) => path.replace(/\\/g, '/').toLowerCase()

const host = (on: On, files: Record<string, string>, runs: string[][], exitCode = 1) => {
  on('session.root', () => ({ value: ROOT }))
  on('fs.read', (_, e) => {
    const text = Object.entries(files).find(([path]) => key(path) === key(e.path))?.[1]
    return text === undefined ? { deny: 'missing' } : { value: text }
  })
  on('fs.write', (_, e) => {
    const known = Object.keys(files).find(path => key(path) === key(e.path)) ?? e.path
    files[known] = e.text
    return { value: undefined }
  })
  on('process.run', (_, e) => {
    runs.push([...e.argv])
    const fileText = files[`${ROOT}/a.go`] ?? ''
    return { value: { exitCode: fileText.includes('<=') ? exitCode : 0, stdout: '--- FAIL: TestA (0s)\n',
      stderr: '', isStdoutTruncated: false, isStderrTruncated: false } }
  })
  on('tool.call', (_, e) => ({ result: { stdout: `ran: ${(e as { command?: string }).command ?? ''}`, stderr: '' } }))
}

test('the mutation tool runs the test on the broken file and puts the original back', async ($, on) => {
  const files: Record<string, string> = { [`${ROOT}/a.go`]: 'if a < b {' }
  const runs: string[][] = []
  host(on, files, runs)
  const call = { tool: 'mcp__skillhub-guards__mutation_probe' as const,
    input: { file: 'a.go', find: '<', replace: '<=', argv: ['go', 'test', './...'] } }
  const result = await $.tool.call(call as never) as { result: string, isError?: boolean }
  expect(result.result).toMatch(/^RED/)
  expect(result.result).toContain('file restored byte for byte')
  expect(runs).toEqual([['go', 'test', './...']])
  expect(files[`${ROOT}/a.go`]).toBe('if a < b {')
})

test('a platform go test is sent to the shell with the test database', async ($, on) => {
  const files: Record<string, string> = { [`${ROOT}/.env`]: `SKILLHUB_TEST_DATABASE_URL=${DSN}\n` }
  host(on, files, [])
  const result = await $.tool.call({ tool: 'Bash', command: 'go -C apps/platform test ./...' }) as
    { result: { stdout: string } }
  expect(result.result.stdout).toContain(`ran: ${ENV} go -C apps/platform test -count=1 ./...`)
  expect(result.result.stdout).toMatch(/^\[skillhub-guards\] ran against the test database/)
})
