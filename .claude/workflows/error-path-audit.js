export const meta = {
  name: 'error-path-audit',
  description: 'Walk one feature line at a time from the Go handler through public.yaml to the page and its tests, list every failure/refusal/absence path, then try to refute each finding before it reaches the ledger',
  phases: [
    { title: 'Read', detail: 'one reader per line, Go + contract + page + tests read in full' },
    { title: 'Refute', detail: 'one skeptic per line re-opens every finding at file:line, default to refuted' },
    { title: 'Critic', detail: 'routes and files no reader covered; cross-line duplicates in shared components' },
  ],
}

// Every subagent gets an explicit model. Bare agent() would inherit the
// dispatcher's flagship model, which 根 AGENTS.md〈開發自動化〉第 3 條 forbids.
const run = (prompt, opts = {}) => agent(prompt, { ...opts, model: opts.model ?? 'sonnet' })

// Feature lines. GenerateSkill.tsx is absent on purpose (01 §10 ⛔ 1). The run
// line (RunPreflight/RunTrace/RunEvaluation/RunCompare) was walked by hand on
// 2026-09-04 and its six gaps are 04 丙-143～148; pass it in args.lines to redo it.
const WEB = 'apps/web/'
const GO = 'apps/platform/internal/'
const DEFAULT_LINES = [
  { key: 'discover', web: ['src/pages/Home.tsx', 'src/pages/Compare.tsx', 'src/components/RiskIndicator.tsx', 'src/components/CompatibilityStatus.tsx', 'src/api/skills.ts'], go: ['skill/discovery/http.go'], tests: ['src/disc.test.tsx', 'src/compare.test.tsx'] },
  { key: 'detail', web: ['src/pages/SkillDetail.tsx', 'src/pages/SkillFiles.tsx', 'src/components/LicenseBadge.tsx', 'src/components/LabelledBadge.tsx'], go: ['skill/discovery/http.go', 'skill/library/http.go', 'creator/workspace/http.go'], tests: ['src/detail.test.tsx', 'src/session.test.tsx'] },
  { key: 'import', web: ['src/pages/ImportSkill.tsx', 'src/pages/WorkspaceSkills.tsx', 'src/components/CreateHub.tsx', 'src/components/VersionUpload.tsx', 'src/components/ConfirmDelete.tsx', 'src/components/Findings.tsx'], go: ['skill/admission/http.go', 'skill/library/http.go'], tests: ['src/workspace.test.tsx'] },
  { key: 'lab', web: ['src/pages/TestCases.tsx', 'src/pages/DatasetUpload.tsx', 'src/api/testcases.ts', 'src/api/lab.ts'], go: ['trial/design/http.go'], tests: ['src/testcases.test.tsx', 'src/dataset.test.tsx'] },
  { key: 'packaging', web: ['src/pages/Packaging.tsx', 'src/pages/Downloads.tsx', 'src/components/DownloadArtifactFacts.tsx', 'src/api/packaging.ts'], go: ['skill/delivery/http.go'], tests: ['src/packaging.test.tsx'] },
  { key: 'account', web: ['src/pages/WorkspaceAccount.tsx', 'src/pages/DataPolicy.tsx', 'src/components/FeedbackEntry.tsx', 'src/components/AuthControls.tsx', 'src/components/SignIn.tsx', 'src/components/CleanModeNotice.tsx', 'src/api/me.ts', 'src/api/feedback.ts'], go: ['creator/workspace/http.go', 'product/learning/feedback.go'], tests: ['src/session.test.tsx', 'src/feedback.test.tsx', 'src/signin.test.tsx', 'src/policy.test.tsx', 'src/clean-mode.test.tsx'] },
]
const lines = (args && args.lines) || DEFAULT_LINES

// The seven shapes the run line taught (04 丙-143～148). A reader hunts these;
// anything else it finds goes under `other` with the rule it breaks.
const SHAPES = [
  '1 english-to-screen: a role=alert/status (or any visible text) that prints err.message, a Go hand-written English note, or a contract constant raw. Includes mutation onError → setMessage(err.message).',
  '2 fixture-lies: a test fixture or stub whose string is not what the Go handler actually sends (typically Chinese in the fixture, English in Go), so the suite is green while production shows the other.',
  '3 contract-drift: a value the hand-written handler emits that public.yaml does not declare (enum, status code, field), or a declared 4xx the page treats generically.',
  '4 absence-wrong: a "no value" rendered blank, as —/N/A, as a word outside system.md §2.9 (0／不適用／未測量／測量失敗／無權檢視／尚未定值), or as a wrong claim (a read failure shown as 「not yours」/「none」).',
  '5 gate-unstated: a limit the server enforces (RequireInvited, quota, size, count, concurrency, license hold) that the page does not state before the user acts (system.md §2.2 第三向, §3 第 11 條; CreateHub.tsx:76-82 is the precedent shape).',
  '6 read-unhandled: a query whose .error is never read, or a 401 on a read/write that does not go through ReadFailure/LoginRequired (IA §5 IA-6).',
  '7 field-dropped: a field the contract declares and the server sends that the page never renders (system.md §3 第 4 條 names status_reason/size_bytes/details), or a failure in role=status / an expected state in role=alert.',
].join('\n')

const FINDING = {
  type: 'object',
  properties: {
    id: { type: 'string', description: 'line key + running number, e.g. lab-3' },
    shape: { type: 'string', enum: ['1', '2', '3', '4', '5', '6', '7', 'other'] },
    file: { type: 'string' },
    line: { type: 'integer' },
    trigger: { type: 'string', description: 'the condition that produces it' },
    server: { type: 'string', description: 'Go file:line, status, exact string or enum value' },
    contract: { type: 'string', description: 'public.yaml location, or "undeclared"' },
    copy: { type: 'string', description: 'exact user-visible text, verbatim' },
    evidence: { type: 'string', description: 'why this is a defect, citing the rule' },
    severity: { type: 'string', enum: ['high', 'medium', 'low'], description: 'high = a beta tester on the main journey sees it; low = reachable only by API misuse' },
    test_gap: { type: 'boolean', description: 'true when no test in the line\'s test files exercises this path' },
    fix: { type: 'string', description: 'the smallest change, one sentence' },
  },
  required: ['id', 'shape', 'file', 'line', 'trigger', 'server', 'contract', 'copy', 'evidence', 'severity', 'test_gap', 'fix'],
}
const READ_SCHEMA = {
  type: 'object',
  properties: {
    line: { type: 'string' },
    files_read: { type: 'array', items: { type: 'string' } },
    paths_walked: { type: 'integer', description: 'how many distinct failure/absence paths you enumerated before filtering to findings' },
    findings: { type: 'array', items: FINDING },
    done_right: { type: 'array', items: { type: 'string' }, description: 'paths handled correctly, one line each with file:line — so a fixer does not break them' },
    unsure: { type: 'array', items: { type: 'string' } },
  },
  required: ['line', 'files_read', 'paths_walked', 'findings', 'done_right', 'unsure'],
}
const VERDICT_SCHEMA = {
  type: 'object',
  properties: {
    line: { type: 'string' },
    verdicts: {
      type: 'array',
      items: {
        type: 'object',
        properties: {
          id: { type: 'string' },
          verdict: { type: 'string', enum: ['CONFIRMED', 'REFUTED', 'PARTLY'] },
          evidence: { type: 'string', description: 'file:line you re-opened and what it actually says' },
          corrected_fix: { type: 'string', description: 'the fix as you would state it after reading, or empty' },
        },
        required: ['id', 'verdict', 'evidence', 'corrected_fix'],
      },
    },
    missed: { type: 'array', items: FINDING, description: 'paths of the seven shapes the reader did not list, found while re-opening the files' },
  },
  required: ['line', 'verdicts', 'missed'],
}

const CONTEXT = [
  'Repo: C:/Users/a8022/OneDrive/Desktop/SkillHub. READ-ONLY: no edits, no git writes. Use Glob/Grep ONLY to locate; analyse by reading whole files with the Read tool.',
  'Rules to judge against (read these sections first): docs/design/system.md §2.1, §2.2, §2.4, §2.9, §2.12 and §3 rows 2/3/4/5/6/11; docs/design/information-architecture.md §5 IA-6; docs/plans/02-specifications-and-acceptance-criteria.md NFR-007.',
  'Conventions already in the code: `ReadFailure`/`LoginRequired` in apps/web/src/components/LoginRequired.tsx (401 → 「需要登入」 + sign-in; otherwise children or 「無法讀取{what}：{message}」); `ApiError` in apps/web/src/api/client.ts carries status and body; Go handlers answer `{"error": "..."}` via httpx.WriteError; `Labelled` {value,label,note} is the served-words shape for enums.',
  'Precedent: 04 丙-143～148 (docs/plans/04-backlog-and-handoffs.md) — read those six rows; the run line was fixed on 2026-09-04 and the same seven shapes are what you hunt on this line.',
  '',
  'The seven shapes:',
  SHAPES,
].join('\n')

phase('Read')
log(`${lines.length} lines; GenerateSkill.tsx excluded (01 §10 ⛔ 1); run line excluded (already 04 丙-143～148)`)

const results = await pipeline(
  lines,
  (l) => run(
    [
      CONTEXT,
      '',
      `LINE ${l.key}. Read IN FULL: ${l.web.map((f) => WEB + f).join(', ')}; Go: ${l.go.map((f) => GO + f).join(', ')}; tests: ${l.tests.map((f) => WEB + f).join(', ')}. Locate the matching paths in contracts/openapi/public.yaml and read those sections (the 4xx responses and the schemas the page renders).`,
      'Task: enumerate every failure / refusal / absence / degraded path on this line — each 4xx/5xx the handler can answer, each enum value, each optional field, each mutation error branch, each empty state — and for each decide whether it is one of the seven shapes. Report only defects as findings (with exact copy and file:line on both sides); list what is done right under done_right so a fixer keeps it. Severity by who sees it: a beta tester on the main journey = high.',
      'Do not report the same defect twice for two call sites of one shared component; report the component once and name the call sites in evidence.',
      'Return the structured object only.',
    ].join('\n'),
    { label: `read:${l.key}`, phase: 'Read', schema: READ_SCHEMA, effort: 'medium' },
  ),
  (r, l) => {
    if (!r) return null
    return run(
      [
        CONTEXT,
        '',
        `Refute the findings below for LINE ${l.key}. For EACH finding: re-open the file at the line (and the Go/contract side it cites) and decide CONFIRMED / REFUTED / PARTLY with the evidence you actually read. Default to REFUTED when the cited line does not say what the finding claims, when an existing test already covers it, or when a rule the finding cites does not apply. Then list any path of the seven shapes the reader missed, found while you were in the files.`,
        '',
        JSON.stringify(r.findings),
        '',
        'Return the structured verdict only.',
      ].join('\n'),
      { label: `refute:${l.key}`, phase: 'Refute', schema: VERDICT_SCHEMA, effort: 'high' },
    ).then((v) => ({ read: r, verdict: v }))
  },
)

const ok = results.filter(Boolean)
if (ok.length < lines.length) log(`${lines.length - ok.length} line(s) returned nothing — NOT covered`)

phase('Critic')
const critic = await run(
  [
    'You are the completeness critic for an error-path audit of apps/web + apps/platform. Lines and the files each was supposed to read:',
    JSON.stringify(lines.map((l) => ({ line: l.key, web: l.web.map((f) => WEB + f), go: l.go.map((f) => GO + f) }))),
    'Files each reader says it read:',
    JSON.stringify(ok.map((x) => ({ line: x.read.line, files_read: x.read.files_read }))),
    '',
    'With Glob/Read only (no grep for text): (1) list every route in docs/design/information-architecture.md §2.1 that no line covers (the run line /lab/run, /runs/$runId, /runs/$runId/compare is covered by 04 丙-143～148 — say so, do not list it); (2) list every .tsx under apps/web/src/pages and apps/web/src/components in no line (GenerateSkill.tsx is excluded on purpose — say so); (3) list every Go http.go/*_http.go under apps/platform/internal in no line; (4) name any finding that appears in two lines for the same shared component so the ledger records it once.',
    'Return a short plain-text list, nothing else.',
  ].join('\n'),
  { label: 'critic', phase: 'Critic', effort: 'medium' },
)

const confirmed = ok.flatMap((x) => {
  const byId = new Map((x.verdict && x.verdict.verdicts || []).map((v) => [v.id, v]))
  return x.read.findings
    .map((f) => ({ ...f, verdict: byId.get(f.id) }))
    .filter((f) => f.verdict && f.verdict.verdict !== 'REFUTED')
})
const missed = ok.flatMap((x) => (x.verdict && x.verdict.missed) || [])
log(`${confirmed.length} findings survived refutation across ${ok.length} lines; ${missed.length} added by refuters`)

return {
  lines: ok.map((x) => ({ line: x.read.line, files_read: x.read.files_read, paths_walked: x.read.paths_walked, done_right: x.read.done_right, unsure: x.read.unsure })),
  confirmed,
  missed,
  critic,
}
