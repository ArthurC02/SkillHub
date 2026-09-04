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
//   briefs: [{ key, paths: [...allowlist], brief, test: 'vitest file or pattern',
//             test_cmd: 'optional full command run from the repo root instead of vitest — for Go briefs:
//                        "go -C apps/platform test ./internal/trial/design/..."' }],
//   context: 'text every writer and verifier gets first',
//   coordinator_files: ['paths the coordinator is editing at the same time — verifiers do not report them'],
//   gate: 'shell command whose success line the gate agent must quote',
//   gate_cwd: 'directory the gate runs in (default apps/web)',
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
const coordinatorFiles = Array.isArray(args.coordinator_files) ? args.coordinator_files : []
const gate = args.gate || 'npm run typecheck && npx vitest run && npm run lint && npm run format:check'
const gateCwd = args.gate_cwd || 'apps/web'
const gateSuccess = args.gate_success || 'All matched files use Prettier code style!'
// A Go gate without the database is a skip that reports ok: three apiserver
// assertions reached CI that way on 2026-09-04. Refuse the shape, not the run.
if (/\bgo\b[^&|;]*\btest\b/.test(gate) && !/SKILLHUB_REQUIRE_DB=1/.test(gate)) {
  throw new Error('a Go gate must set SKILLHUB_REQUIRE_DB=1 (and SKILLHUB_TEST_DATABASE_URL) so skipped DB tests fail instead of passing')
}
// Every path another brief owns, or the coordinator owns, is an expected change
// in the shared tree — not a finding for the verifier of this brief.
const expectedElsewhere = (key) => [
  ...coordinatorFiles,
  ...args.briefs.filter((o) => o.key !== key).flatMap((o) => o.paths),
]
const isExpected = (file, key) => expectedElsewhere(key).some((p) => file === p || file.startsWith(p))
// The mutation agent's "red" counts only when the failure output carries an
// assertion: a Go `x_test.go:NN:` line or a vitest AssertionError/expected. A
// missing file, module or `document` is the test not running.
const ASSERTION = /_test\.go:\d+:|AssertionError|expected .* to |toBe|toContain|toEqual|Error: expect/
const assertionRed = (m) => Boolean(m && m.went_red && ASSERTION.test(m.red_output_head || ''))

const WRITE_RESULT = {
  type: 'object',
  properties: {
    files_changed: { type: 'array', items: { type: 'string' } },
    summary: { type: 'string' },
    stopped_because: { type: 'string', description: 'non-empty ONLY when your own brief cannot be done as written and you changed nothing; a problem in a file outside your allowlist goes in summary, not here' },
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
    red_assertion_line: { type: 'string', description: 'the one line of failure output that is the assertion (Go: "file_test.go:NN: ..."; vitest: the AssertionError line); empty if there was none — then went_red must be false' },
    restored: { type: 'boolean', description: 'git diff of the file equals the pre-mutation diff' },
  },
  required: ['went_red', 'red_output_head', 'red_assertion_line', 'restored'],
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
      `BRIEF ${b.key}. You may edit ONLY these paths: ${b.paths.join(', ')}. Any other file you believe must change: do not edit it — finish your own brief and name that file and why in summary. stopped_because is for one case only: your own brief cannot be done as written (it contradicts the code inside your allowlist) and you changed nothing; a neighbour's problem is never a reason to stop. No git commands that write (add/commit/stash/reset/checkout/restore), no formatter over the repo, no package install.`,
      '',
      b.brief,
      '',
      b.test_cmd
        ? `When done, run from the repo root: ${b.test_cmd} — redirect the output to a file in the scratchpad and read it, then quote the pass line. Then name the single source line whose removal would make that test red (you will be checked).`
        : `When done, run inside apps/web: npx vitest run ${b.test} — and quote the pass line. Then name the single source line whose removal would make that test red (you will be checked).`,
      'Read the files you edit in full before editing. If the brief contradicts what the code actually does, stop and report; do not follow the brief into the code.',
    ].join('\n'),
    { label: `write:${b.key}`, phase: 'Write', schema: WRITE_RESULT, agentType: 'general-purpose', effort: 'medium' },
  ),
  (w, b) => {
    if (!w) return null
    // A writer that changed files did not stop, whatever it wrote in
    // stopped_because (2026-09-04: a neighbour-file complaint there skipped the
    // verify and mutation of a brief that had landed, and the leak it carried
    // was found only because the coordinator sent a second verifier).
    if (w.stopped_because && w.files_changed.length > 0) {
      log(`${b.key}: writer changed ${w.files_changed.length} files but also wrote stopped_because — treating as landed, verifier told: ${w.stopped_because}`)
    } else if (w.stopped_because) { log(`${b.key} stopped: ${w.stopped_because}`); return { w, v: null } }
    const expected = expectedElsewhere(b.key)
    return run(
      [
        context,
        '',
        `Review brief ${b.key} read-only. Allowed paths: ${b.paths.join(', ')}. Run git diff --name-only and git diff -- <those paths>. Report any changed file outside the allowlist EXCEPT these, which other writers or the coordinator are editing in the same tree on purpose: ${expected.length ? expected.join(', ') : '(none)'}. Hold the diff against the brief and against docs/design/system.md §2.13 (Tip six conditions, A/B/C never folded) and §4.7; report each break as one line with file:line. Do not edit anything.`,
        '',
        'The brief was:',
        b.brief,
        '',
        'The writer reported:',
        JSON.stringify(w),
      ].join('\n'),
      { label: `verify:${b.key}`, phase: 'Verify', schema: VERIFY_RESULT, agentType: 'general-purpose', effort: 'medium' },
    ).then((v) => {
      if (!v) return { w, v }
      // The verifier may still list expected files; drop them here so the
      // out_of_scope list in the result means what it says.
      const kept = v.out_of_scope_files.filter((f) => !isExpected(f, b.key))
      if (kept.length !== v.out_of_scope_files.length) log(`${b.key}: ${v.out_of_scope_files.length - kept.length} out-of-scope files were other briefs' or the coordinator's; dropped`)
      return { w, v: { ...v, out_of_scope_files: kept } }
    })
  },
  (r, b) => {
    if (!r || !r.v || !r.v.ok) return r
    // A brief whose only machine is coordinator-owned (an e2e ratchet, say) has
    // nothing a writer can make red; say so in the log instead of pretending.
    if (!b.test && !b.test_cmd) { log(`${b.key}: no writer-owned test named, mutation left to the coordinator`); return r }
    const runTest = b.test_cmd
      ? `run from the repo root: ${b.test_cmd} (redirect to a scratchpad file and read it)`
      : 'run npx vitest run ' + r.w.test_file.replace(/^apps\/web\//, '') + ' from inside apps/web'
    return run(
      [
        `Mutation check for brief ${b.key}. The writer says test ${r.w.test_file} goes red when this line is removed: ${r.w.red_line}.`,
        `Do exactly this: (1) record git diff -- <file> for the file holding that line; (2) remove or neuter that one line with the Edit tool; (3) ${runTest} and capture the head of the failure output; (4) restore the line with Edit; (5) run git diff -- <file> again and confirm it equals step 1 byte for byte.`,
        'If the test stays green after step 2, report went_red=false — that is the finding. went_red=true requires an assertion line in the failure output, and you copy that one line into red_assertion_line (Go: "something_test.go:NN: ..."; vitest: the AssertionError line). A red that is not an assertion — "document is not defined", "no test files found", a module that cannot be resolved — is NOT red: it means the test did not run (wrong directory or wrong path); fix the invocation and run again. Never leave the file mutated. No git writes.',
      ].join('\n'),
      { label: `mutate:${b.key}`, phase: 'Mutate', schema: MUTATION_RESULT, agentType: 'general-purpose', model: 'haiku', effort: 'low' },
    ).then((m) => {
      // Judged here, not by the agent: went_red is true only with an assertion.
      const verified = assertionRed(m)
      if (m && m.went_red && !verified) log(`${b.key}: mutation reported red without an assertion line — counted as NOT red: ${(m.red_output_head || '').slice(0, 120)}`)
      return { ...r, m: m ? { ...m, went_red: verified, reported_went_red: m.went_red } : m }
    })
  },
)

phase('Gate')
const landed = results.filter((r) => r && r.v && r.v.ok)
log(`${landed.length}/${args.briefs.length} briefs landed and verified; running the gate once`)
const gateResult = landed.length === 0 ? null : await run(
  [
    `Run this from ${gateCwd} (cd there first) and report: ${gate}`,
    `Passing means the exit code is 0 AND you saw this exact line: ${gateSuccess}. Redirect output to a file in the scratchpad and read the file — the terminal output is filtered. Quote the last 15 lines. Do not fix anything, do not edit files.`,
  ].join('\n'),
  { label: 'gate', phase: 'Gate', schema: GATE_RESULT, agentType: 'general-purpose', model: 'haiku', effort: 'low' },
)

return {
  briefs: results.map((r, i) => ({ key: args.briefs[i].key, ...(r || { w: null, v: null }) })),
  gate: gateResult,
}
