export const DOCUMENTED_TEST_DSN = 'postgres://skillhub:skillhub@localhost:5432/skillhub_test?sslmode=disable'
export const CI_POLL_MS = 60_000
export const CI_GIVE_UP_MS = 60 * 60_000
export const MUTATION_TIMEOUT_MS = 600_000
const SUMMARY_LINES = 30
export const MUTATION_TOOL = 'mutation_probe'
export const TEST_DATABASE_NOTE = '[skillhub-guards] ran against the test database with SKILLHUB_REQUIRE_DB=1 and -count=1\n'

const ARG = String.raw`(?:"[^"]*"|'[^']*'|\S+)`
const GO_TEST = new RegExp(
  String.raw`(^|[;&|(\n]|\$\()(\s*)((?:[A-Za-z_]\w*=\S*\s+)*)((?:rtk\s+(?:proxy\s+)?)?go(?:\s+-C\s+${ARG})?\s+test\b)`)
const FAILURE_LINE = /--- FAIL|^FAIL\b|^panic:|\(fail\)|FAILED|AssertionError|✗|×|Error:/
const BUILD_FAILURE = /\[build failed\]|\[setup failed\]|error TS\d+|SyntaxError|cannot find package|\bundefined: /

export const dsnFromEnvFile = (text: string): string | undefined =>
  text.match(/^SKILLHUB_TEST_DATABASE_URL=["']?([^"'\r\n]+)["']?\s*$/m)?.[1]

export const withTestDatabase = (command: string, dsn: string): string | undefined => {
  if (!/apps[\\/]platform/.test(command) || /SKILLHUB_TEST_DATABASE_URL=/.test(command)) return undefined
  const match = GO_TEST.exec(command)
  if (!match) return undefined
  const [whole, separator, space, assignments, goTest] = match
  const count = /\s-count[= ]/.test(command) ? '' : ' -count=1'
  const rewritten = `${separator}${space}${assignments}SKILLHUB_TEST_DATABASE_URL='${dsn}' SKILLHUB_REQUIRE_DB=1 ${goTest}${count}`
  return command.slice(0, match.index) + rewritten + command.slice(match.index + whole.length)
}

export type CIVerdict = 'green' | 'red' | 'pending'

export const ciVerdict = (output: string): CIVerdict | undefined =>
  output.match(/^\w+ \(\w+\): (green|red|pending)\b/m)?.[1] as CIVerdict | undefined

export const replaceOnce = (text: string, find: string, replace: string): string | { problem: string } => {
  const first = text.indexOf(find)
  if (find === '' || first < 0) return { problem: 'the text to break was not found in the file' }
  if (text.indexOf(find, first + 1) >= 0) return { problem: 'the text to break occurs more than once; give a longer snippet' }
  return text.slice(0, first) + replace + text.slice(first + find.length)
}

export const mutationVerdict = (exitCode: number, output: string) => {
  const lines = output.split(/\r?\n/)
  const failures = lines.filter(line => FAILURE_LINE.test(line))
  const summary = (failures.length > 0 ? failures : lines.filter(line => line.trim() !== ''))
    .slice(-SUMMARY_LINES).join('\n')
  if (exitCode === 0) return { verdict: 'GREEN: the test still passes with the line broken; it does not catch this', summary }
  if (BUILD_FAILURE.test(output)) {
    return { verdict: 'BUILD FAILED: a compile error does not prove the test catches it; break the behaviour instead', summary }
  }
  return { verdict: 'RED: the test fails with the line broken', summary }
}

export const MUTATION_TOOL_SPEC = {
  name: MUTATION_TOOL,
  description: 'Prove a test catches a defect: replaces one exact snippet in a product file, runs the given ' +
    'test command, always restores the file byte for byte, and reports RED, GREEN or BUILD FAILED with the ' +
    'failing lines. Use it for every "fixed X" claim instead of editing and reverting by hand.',
  inputSchema: {
    type: 'object',
    properties: {
      file: { type: 'string', description: 'product file, absolute or relative to the project root' },
      find: { type: 'string', description: 'exact snippet that occurs once in the file' },
      replace: { type: 'string', description: 'the broken version of that snippet' },
      argv: { type: 'array', items: { type: 'string' }, description: 'test command as argv, no shell' },
      cwd: { type: 'string', description: 'directory to run in, relative to the project root' },
      env: { type: 'object', additionalProperties: { type: 'string' } },
    },
    required: ['file', 'find', 'replace', 'argv'],
  },
}

export type MutationInput = {
  file: string
  find: string
  replace: string
  argv: string[]
  cwd?: string
  env?: Record<string, string>
}

export const ciMessage = (sha: string, verdict: 'green' | 'red', output: string): string => {
  const failed = output.split('\n').filter(line => /^\s+failure /.test(line)).map(line => line.trim())
  return verdict === 'green' ? `CI ${sha.slice(0, 8)}: green` : `CI ${sha.slice(0, 8)}: red — ${failed.join('; ') || 'see ci-status'}`
}
