export const meta = {
  name: 'parallel-page-edit',
  description: 'Fan out disjoint-path writer briefs, verify each diff against its brief, prove one test per brief goes red, then run the repo gate once',
  phases: [
    { title: 'Write', detail: 'one writer per brief, paths disjoint, no git writes' },
    { title: 'Verify', detail: 'a read-only reviewer holds the diff against the brief and the design rules' },
    { title: 'Mutate', detail: 'revert the brief\'s fix, run its test, expect red, restore' },
    { title: 'Gate', detail: 'the repo\'s own gate, once, after every brief has landed' },
  ],
}

// Every subagent gets an explicit model. Bare agent() would inherit the
// dispatcher's flagship model, which 根 AGENTS.md〈開發自動化〉第 3 條 forbids.
const run = (prompt, opts = {}) => agent(prompt, { ...opts, model: opts.model ?? 'sonnet' })

// args: {
//   briefs: [{ key, paths: [...allowlist], brief, test: 'vitest file or pattern' }],
//   context: 'text every writer and verifier gets first',
//   gate: 'shell command whose success line the gate agent must quote',
//   gate_success: 'the literal line that means the gate passed',
// }
// Paths must be disjoint across briefs — the script checks and refuses.
if (!args || !Array.isArray(args.briefs) || args.briefs.length === 0) throw new Error('args.briefs is required')
const seen = new Map()
for (const b of args.briefs) for (const p of b.paths) {
  if (seen.has(p)) throw new Error(`path ${p} is in two briefs (${seen.get(p)}, ${b.key}); one writer per file`)
  seen.set(p, b.key)
}
const context = args.context || ''
const gate = args.gate || 'npm run typecheck && npx vitest run && npm run lint && npm run format:check'
const gateSuccess = args.gate_success || 'All matched files use Prettier code style!'

const WRITE_RESULT = {
  type: 'object',
  properties: {
    files_changed: { type: 'array', items: { type: 'string' } },
    summary: { type: 'string' },
    stopped_because: { type: 'string', description: 'non-empty when the brief conflicted with the code and you stopped' },
    test_file: { type: 'string', description: 'the test that would go red if your change were reverted' },
    red_line: { type: 'string', description: 'the exact source line whose removal makes that test red' },
  },
  required: ['files_changed', 'summary', 'stopped_because', 'test_file', 'red_line'],
}
const VERIFY_RESULT = {
  type: 'object',
  properties: {
    ok: { type: 'boolean' },
    out_of_scope_files: { type: 'array', items: { type: 'string' } },
    rule_breaks: { type: 'array', items: { type: 'string' } },
    notes: { type: 'string' },
  },
  required: ['ok', 'out_of_scope_files', 'rule_breaks', 'notes'],
}
const MUTATION_RESULT = {
  type: 'object',
  properties: {
    went_red: { type: 'boolean' },
    red_output_head: { type: 'string' },
    restored: { type: 'boolean', description: 'git diff of the file equals the pre-mutation diff' },
  },
  required: ['went_red', 'red_output_head', 'restored'],
}
const GATE_RESULT = {
  type: 'object',
  properties: {
    passed: { type: 'boolean' },
    success_line_seen: { type: 'boolean' },
    exit_code: { type: 'integer' },
    tail: { type: 'string' },
  },
  required: ['passed', 'success_line_seen', 'exit_code', 'tail'],
}

log(`${args.briefs.length} briefs, paths disjoint; gate: ${gate}`)

const results = await pipeline(
  args.briefs,
  (b) => run(
    [
      context,
      '',
      `BRIEF ${b.key}. You may edit ONLY these paths: ${b.paths.join(', ')}. Any other file you believe must change: stop and report it in stopped_because instead of editing. No git commands that write (add/commit/stash/reset/checkout/restore), no formatter over the repo, no package install.`,
      '',
      b.brief,
      '',
      `When done, run: npx vitest run ${b.test} — and quote the pass line. Then name the single source line whose removal would make that test red (you will be checked).`,
      'Read the files you edit in full before editing. If the brief contradicts what the code actually does, stop and report; do not follow the brief into the code.',
    ].join('\n'),
    { label: `write:${b.key}`, phase: 'Write', schema: WRITE_RESULT, agentType: 'general-purpose', effort: 'medium' },
  ),
  (w, b) => {
    if (!w) return null
    if (w.stopped_because) { log(`${b.key} stopped: ${w.stopped_because}`); return { w, v: null } }
    return run(
      [
        context,
        '',
        `Review brief ${b.key} read-only. Allowed paths: ${b.paths.join(', ')}. Run git diff --name-only and git diff -- <those paths>. Report any changed file outside the allowlist. Hold the diff against the brief and against docs/design/system.md §2.13 (Tip six conditions, A/B/C never folded) and §4.7; report each break as one line with file:line. Do not edit anything.`,
        '',
        'The brief was:',
        b.brief,
        '',
        'The writer reported:',
        JSON.stringify(w),
      ].join('\n'),
      { label: `verify:${b.key}`, phase: 'Verify', schema: VERIFY_RESULT, agentType: 'general-purpose', effort: 'medium' },
    ).then((v) => ({ w, v }))
  },
  (r, b) => {
    if (!r || !r.v || !r.v.ok) return r
    // A brief whose only machine is coordinator-owned (an e2e ratchet, say) has
    // nothing a writer can make red; say so in the log instead of pretending.
    if (!b.test) { log(`${b.key}: no writer-owned test named, mutation left to the coordinator`); return r }
    return run(
      [
        `Mutation check for brief ${b.key}. The writer says test ${r.w.test_file} goes red when this line is removed: ${r.w.red_line}.`,
        'Do exactly this: (1) record git diff -- <file> for the file holding that line; (2) remove or neuter that one line with the Edit tool; (3) run npx vitest run ' + r.w.test_file + ' and capture the head of the failure output; (4) restore the line with Edit; (5) run git diff -- <file> again and confirm it equals step 1 byte for byte.',
        'If the test stays green after step 2, report went_red=false — that is the finding. Never leave the file mutated. No git writes.',
      ].join('\n'),
      { label: `mutate:${b.key}`, phase: 'Mutate', schema: MUTATION_RESULT, agentType: 'general-purpose', model: 'haiku', effort: 'low' },
    ).then((m) => ({ ...r, m }))
  },
)

phase('Gate')
const landed = results.filter((r) => r && r.v && r.v.ok)
log(`${landed.length}/${args.briefs.length} briefs landed and verified; running the gate once`)
const gateResult = landed.length === 0 ? null : await run(
  [
    `Run this in apps/web and report: ${gate}`,
    `Passing means the exit code is 0 AND you saw this exact line: ${gateSuccess}. Redirect output to a file in the scratchpad and read the file — the terminal output is filtered. Quote the last 15 lines. Do not fix anything, do not edit files.`,
  ].join('\n'),
  { label: 'gate', phase: 'Gate', schema: GATE_RESULT, agentType: 'general-purpose', model: 'haiku', effort: 'low' },
)

return {
  briefs: results.map((r, i) => ({ key: args.briefs[i].key, ...(r || { w: null, v: null }) })),
  gate: gateResult,
}
