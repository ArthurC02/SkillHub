export const meta = {
  name: 'ux-text-audit',
  description: 'Classify every visible sentence in apps/web by 設計 §2.13 class (A/B/C/H/G/D/F/E), count runes, list D blocks, Tip and icon candidates; then try to refute the classification',
  phases: [
    { title: 'Read', detail: 'one reader per page group, source files read in full' },
    { title: 'Refute', detail: 'three skeptics attack the D/C boundary and the Tip candidates' },
    { title: 'Critic', detail: 'what was not read, what cannot be counted' },
  ],
}

// Every subagent gets an explicit model. Bare agent() would inherit the
// dispatcher's flagship model, which 根 AGENTS.md〈開發自動化〉第 3 條 forbids.
// The harness checker counts the `agent(` calls in this file and expects each
// on a line that also names `model:`.
const run = (prompt, opts = {}) => agent(prompt, { ...opts, model: opts.model ?? 'sonnet' })

// Page groups: one reader each. GenerateSkill.tsx is deliberately absent — the
// M5 entry is behind a flag closed-beta users must not see (01 §10 ⛔ 1), and an
// audit that lists its text invites someone to "improve" it.
const DEFAULT_GROUPS = [
  { key: 'home', files: ['src/pages/Home.tsx'] },
  { key: 'detail', files: ['src/pages/SkillDetail.tsx', 'src/components/LabelledBadge.tsx', 'src/components/LicenseBadge.tsx', 'src/components/RiskIndicator.tsx'] },
  { key: 'packaging', files: ['src/pages/Packaging.tsx', 'src/components/DownloadArtifactFacts.tsx'] },
  { key: 'evaluation', files: ['src/pages/RunEvaluation.tsx', 'src/components/RunVerdict.tsx', 'src/components/Findings.tsx', 'src/components/FeedbackEntry.tsx'] },
  { key: 'trace', files: ['src/pages/RunTrace.tsx', 'src/components/InFlight.tsx', 'src/pages/RunCompare.tsx'] },
  { key: 'testcases', files: ['src/pages/TestCases.tsx', 'src/pages/DatasetUpload.tsx'] },
  { key: 'preflight', files: ['src/pages/RunPreflight.tsx', 'src/components/CleanModeNotice.tsx', 'src/components/CompatibilityStatus.tsx'] },
  { key: 'compare-files-policy', files: ['src/pages/Compare.tsx', 'src/pages/SkillFiles.tsx', 'src/pages/DataPolicy.tsx', 'src/pages/Downloads.tsx'] },
  { key: 'workspace', files: ['src/pages/WorkspaceAccount.tsx', 'src/pages/WorkspaceRuns.tsx', 'src/pages/WorkspaceSkills.tsx', 'src/pages/ImportSkill.tsx', 'src/components/VersionUpload.tsx', 'src/components/ConfirmDelete.tsx', 'src/components/CreateHub.tsx', 'src/components/LoginRequired.tsx', 'src/components/SignIn.tsx', 'src/components/GeneratedNotice.tsx', 'src/components/Timestamp.tsx', 'src/components/Loading.tsx'] },
]
const groups = (args && args.groups) || DEFAULT_GROUPS
const WEB = 'apps/web/'

const RUBRIC = [
  '分類表（docs/design/system.md §2.13，逐字）：',
  'A 判斷依據：§2.10 十項；權限、額度、期限與判定。不入預算。',
  'B 原因：停用、缺席、被擋的原因（§2.4、§2.9）；aria-describedby 的目標。不入預算。',
  'C 但書：「這個徽章不涵蓋什麼」（§2.11(c)）、限制的強制者（§2.2 第三向）。不入預算。',
  'H 限制：上限、白名單、保存天數——平台真的會擋的值。不入預算（值與型別詞不動；推導可移形）。',
  'G 進行中：§2.12 的三件事、會變的量、多久沒動了。不入預算（會變的量永遠平鋪，不會變的理由才可以折）。',
  'D 說明：讀過一次就懂的教學。判準：同一個人第二次來看，這段文字不改變他這次要按什麼。入預算。',
  'F 識別符與推導：雜湊、UUID、digest、映像名、「為什麼是這個順序」。入預算（§2.6 本來就要折）。',
  'E 操作回饋：按鈕字、role="status" 的一句話。不計。',
  '',
  '字數：Unicode code point（[...s].length），與 FeedbackEntry.tsx 的 runes() 同。',
  '預算：路由預設狀態（已登入、讀取成功、清單一列、未展開任何確認、旗標關）下，平鋪的 D＋F ≤ 同頁 A＋B＋C，且單一 D 區塊 ≤ 100 字。',
  '',
  'Tip 入場六條（缺一不可）：只准裝 D 與 F；不得是 title=、不得 hover-only；錨點必須自己成立（被限定的東西本身，或帶主詞／帶數量的短語——「詳情」「說明」「?」不合格）；觸發鈕永遠帶可見文字；一頁至多三個；需要很多 Tip 是設計訊號不是解法。',
  '判準：把一頁上所有 Tip 刪掉之後，每一個判斷仍然做得出來。',
  '',
  '圖示（§4.7）：只出現在 §4.4 狀態語彙的列（不通過／未知或未檢查／降級／通過）與 Tip 觸發鈕；永遠伴隨可見文字；至多六個形狀。',
].join('\n')

const SEGMENT = {
  type: 'object',
  properties: {
    file: { type: 'string' },
    line: { type: 'integer' },
    text: { type: 'string', description: 'first 60 runes of the visible text, verbatim' },
    runes: { type: 'integer' },
    cls: { type: 'string', enum: ['A', 'B', 'C', 'H', 'G', 'D', 'F', 'E'] },
    container: { type: 'string', description: 'e.g. p.note, li, details, title=, notice, h2' },
    default_visible: { type: 'boolean', description: 'rendered flat in the default state (not inside details/hidden/flag)' },
    why: { type: 'string', description: 'one sentence: which rubric line decided the class' },
  },
  required: ['file', 'line', 'text', 'runes', 'cls', 'container', 'default_visible', 'why'],
}

const READ_SCHEMA = {
  type: 'object',
  properties: {
    group: { type: 'string' },
    files_read: { type: 'array', items: { type: 'string' } },
    segments: { type: 'array', items: SEGMENT },
    totals_default_visible: {
      type: 'object',
      description: 'runes per class, only default_visible segments',
      properties: { A: { type: 'integer' }, B: { type: 'integer' }, C: { type: 'integer' }, H: { type: 'integer' }, G: { type: 'integer' }, D: { type: 'integer' }, F: { type: 'integer' }, E: { type: 'integer' } },
      required: ['A', 'B', 'C', 'H', 'G', 'D', 'F', 'E'],
    },
    d_blocks_over_100: { type: 'array', items: { type: 'object', properties: { file: { type: 'string' }, line: { type: 'integer' }, runes: { type: 'integer' }, text: { type: 'string' } }, required: ['file', 'line', 'runes', 'text'] } },
    tip_candidates: {
      type: 'array',
      items: {
        type: 'object',
        properties: {
          file: { type: 'string' },
          line: { type: 'integer' },
          anchor: { type: 'string', description: 'the visible trigger text, must stand on its own' },
          content: { type: 'string', description: 'what goes inside, D or F only' },
          runes_saved: { type: 'integer' },
          six_conditions: { type: 'string', description: 'how each of the six is met, or which fails' },
        },
        required: ['file', 'line', 'anchor', 'content', 'runes_saved', 'six_conditions'],
      },
    },
    icon_sites: { type: 'array', items: { type: 'object', properties: { file: { type: 'string' }, line: { type: 'integer' }, badge_class: { type: 'string' }, state_row: { type: 'string', enum: ['不通過', '未知或未檢查', '降級', '通過', 'none'] }, visible_text: { type: 'string' } }, required: ['file', 'line', 'badge_class', 'state_row', 'visible_text'] } },
    title_attrs: { type: 'array', items: { type: 'object', properties: { file: { type: 'string' }, line: { type: 'integer' }, text: { type: 'string' } }, required: ['file', 'line', 'text'] } },
    unsure: { type: 'array', items: { type: 'string' }, description: 'segments whose class you could not decide, with both candidates' },
  },
  required: ['group', 'files_read', 'segments', 'totals_default_visible', 'd_blocks_over_100', 'tip_candidates', 'icon_sites', 'title_attrs', 'unsure'],
}

phase('Read')
log(`${groups.length} page groups; GenerateSkill.tsx excluded on purpose (01 §10 ⛔ 1)`)

const reads = await pipeline(
  groups,
  (g) => run(
    [
      `You are auditing visible UI text in the SkillHub web app. Read these files IN FULL with the Read tool (never grep for text): ${g.files.map((f) => WEB + f).join(', ')}.`,
      'Also read docs/design/system.md section 2.13 (search the heading "### 2.13") before classifying — the rubric below is a digest of it.',
      '',
      RUBRIC,
      '',
      'Task: list EVERY string a user can see in the browser (JSX text, string literals rendered, aria-label is NOT visible, title= is NOT visible but must be listed separately). Skip code comments. For each: file, line, first 60 runes verbatim, rune count, class, container, whether it is rendered flat in the default state, and one sentence on which rubric line decided the class.',
      'Hard rules: a sentence that names a value the platform enforces is H, not D. A sentence that says what a badge does not cover is C, not D. A count that changes while you watch is G. If two classes fit, put it in `unsure` with both and pick the higher-priority one (A>B>C>H>G>D>F) for the segment.',
      'Then: sum runes per class over default-visible segments; list D blocks over 100 runes; propose Tip candidates only where all six conditions hold and say how; list every badge that maps to a §4.4 state row (that is where an icon may go) with its visible text; list every title= attribute.',
      'Return the structured object. No prose outside it.',
    ].join('\n'),
    { label: `read:${g.key}`, phase: 'Read', schema: READ_SCHEMA, effort: 'medium' },
  ),
)

const readOk = reads.filter(Boolean)
if (readOk.length < groups.length) log(`${groups.length - readOk.length} reader(s) returned nothing — those groups are NOT covered`)

// Refutation needs the whole picture (it compares boundaries across groups), so
// this is one of the two places a barrier is right.
const allSegments = readOk.flatMap((r) => r.segments.map((s) => ({ ...s, group: r.group })))
const allTips = readOk.flatMap((r) => r.tip_candidates.map((t) => ({ ...t, group: r.group })))
const dSegments = allSegments.filter((s) => s.cls === 'D' && s.default_visible)
const cSegments = allSegments.filter((s) => s.cls === 'C' && s.default_visible)
log(`${allSegments.length} segments, ${dSegments.length} default-visible D, ${cSegments.length} default-visible C, ${allTips.length} Tip candidates`)

phase('Refute')
const VERDICT = {
  type: 'object',
  properties: {
    overturned: { type: 'array', items: { type: 'object', properties: { file: { type: 'string' }, line: { type: 'integer' }, from: { type: 'string' }, to: { type: 'string' }, why: { type: 'string' } }, required: ['file', 'line', 'from', 'to', 'why'] } },
    tips_rejected: { type: 'array', items: { type: 'object', properties: { file: { type: 'string' }, line: { type: 'integer' }, which_condition: { type: 'string' }, why: { type: 'string' } }, required: ['file', 'line', 'which_condition', 'why'] } },
    upheld_count: { type: 'integer' },
  },
  required: ['overturned', 'tips_rejected', 'upheld_count'],
}
const lenses = [
  { key: 'C-vs-D', ask: 'Every D below might really be C (a caveat about what a badge or claim does not cover) or H (a value the platform enforces). Open each file at the line, read the surrounding block, and overturn any D that a §2.11(c) or §2.2 reading would protect. Default to overturning when unsure.' },
  { key: 'A-in-Tip', ask: 'Every Tip candidate below might smuggle a §2.10 item (read system.md "### 2.10" first) or fail one of the six conditions — especially anchors that do not stand on their own and content a first-time visitor needs to decide what to press. Reject any candidate that fails; default to rejecting when unsure.' },
  { key: 'runes', ask: 'For ten of the segments below, chosen to cover every group, re-open the file at the line and recount the runes of the full visible string with [...s].length semantics (write a tiny node -e script if needed). Report any count off by more than 10%, and any listed segment that is not actually rendered in the default state.' },
]
const verdicts = await parallel(lenses.map((l) => () => run(
  [
    l.ask,
    '',
    'D segments (default-visible):',
    JSON.stringify(dSegments.map(({ file, line, text, runes, why }) => ({ file: WEB + file, line, text, runes, why }))),
    '',
    'Tip candidates:',
    JSON.stringify(allTips.map(({ file, line, anchor, content, six_conditions }) => ({ file: WEB + file, line, anchor, content, six_conditions }))),
    '',
    'Return the structured verdict only.',
  ].join('\n'),
  { label: `refute:${l.key}`, phase: 'Refute', schema: VERDICT, effort: 'high' },
)))

phase('Critic')
const critic = await run(
  [
    'You are the completeness critic for a UI text audit of apps/web/src. Files that were supposed to be read, by group:',
    JSON.stringify(groups.map((g) => ({ group: g.key, files: g.files.map((f) => WEB + f) }))),
    'Files each reader says it read:',
    JSON.stringify(readOk.map((r) => ({ group: r.group, files_read: r.files_read }))),
    '',
    'Do three things with Glob/Read (no grep for text): (1) list every .tsx under apps/web/src/pages and apps/web/src/components that is in no group at all (GenerateSkill.tsx is excluded on purpose — say so, do not list it as missing); (2) for each group, name any file listed but not in files_read; (3) name text that no reader can classify from source alone — strings the server sends (e.g. rank_note, notes[], tier.note) — and where the fixture for them lives (apps/web/src/fixtures/platform.ts) so the sums can be completed from real strings.',
    'Return a short plain-text list, nothing else.',
  ].join('\n'),
  { label: 'critic', phase: 'Critic', effort: 'medium' },
)

return {
  groups: readOk,
  refutations: verdicts.filter(Boolean),
  critic,
  totals: readOk.reduce((acc, r) => {
    for (const k of Object.keys(r.totals_default_visible)) acc[k] = (acc[k] || 0) + r.totals_default_visible[k]
    return acc
  }, {}),
}
