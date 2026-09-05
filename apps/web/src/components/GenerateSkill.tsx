import { useEffect, useRef, useState, type ChangeEvent } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { Link } from "@tanstack/react-router";
import { ApiError } from "../api/client";
import { useGenerateFailures, useGenerateSkill } from "../api/generate";
import { isCategorizedFindings } from "../api/import";
import { useSkillSearch } from "../api/skills";
import { useOwnSkills } from "../api/testcases";
import type { GenerateDiagram, GenerateRejected } from "../api/types";
import { Findings } from "./Findings";
import { GeneratedNotice } from "./GeneratedNotice";
import { failureSentence } from "./generateFailureSentence";
import { Timestamp } from "./Timestamp";

/**
 * GEN-004's entry point: describe the task, get a Skill package (GEN-001).
 *
 * Rendered in two places and nowhere else — the search's no-results state and
 * the workspace's own Skill list. **Not on the home page beside search**:
 * ADR-046 決策 7 makes 「先搜尋、搜不到再生成」 a product opinion, and an entry
 * point of equal weight next to the search box says the opposite.
 *
 * `useGenerateEntryPoint` is what decides whether this exists at all. Everything
 * here assumes the flag is already on.
 */

/**
 * The three bounds a generation is held to, stated before the button is
 * pressed (02:GEN-001 「生成前顯示…本次將消耗的額度」, design system §2.2).
 *
 * These are not estimates and they are not the allowance. They are the ceilings
 * the server enforces — ErrGenerateTooLong in admission, finish_reason=length in
 * apps/llm, the retry loop's upper bound — and each carries a one-number marker
 * so `devctl automation-check` fails the day a copy drifts from the line that
 * enforces it. That marker is the answer to §3 第 11 條 「指出強制它的那一行」:
 * the line is named, and a machine compares against it.
 *
 * The textarea deliberately carries no `maxLength`: the browser counts UTF-16
 * code units and the server counts runes, so the same number would refuse an
 * emoji-heavy description the server accepts. One enforcer, and it is the
 * server; the 422 already says what to trim.
 *
 * What is NOT here is a dollar figure. The only unenforced input to a cost is
 * the unit price, which is the provider's fact and changes; printing the
 * measured average ($0.0055) as 「預估成本」 would be the promise §2.2 forbids
 * (04 丙-53). The allowance half is absent too, for the reason RunPreflight's
 * quota row is: a deployment enforcing none sends none, and a number would be a
 * claim (04 乙-2).
 */
const GENERATE_MAX_TASK_RUNES = 4000; // one-number: generateMaxTaskRunes
const GENERATE_MAX_OUTPUT_TOKENS = 16000; // one-number: generateMaxOutputTokens
const GENERATE_MAX_ATTEMPTS = 2; // one-number: generateMaxAttempts

/**
 * The estimate, in the one form 02:PDM-005 §5.3 accepts and RunPreflight already
 * uses: a sourced range with a basis line, labelled an estimate rather than a
 * quote. 04 乙-2 bars an unenforced number from the screen; this clears it the
 * same way preflight.go's three constants do -- it says where it came from, it
 * says it is not a price, and it is not part of any hash.
 *
 * Measured, not modelled: ten generations through this exact path (endpoint ->
 * LiteLLM -> OpenAI, mini tier, strict schema) on 2026-08-25 gave min $0.0038,
 * median $0.0062, max $0.0110, every one of them priced by the gateway rather
 * than estimated (m5/report-generate-baseline.md §8.2).
 *
 * The published high end is wider than that sample, and deliberately: all ten
 * were single-attempt, and GENERATE_MAX_ATTEMPTS is 2, so a retried generation
 * pays twice. Ten calls is not a bound either. What it is NOT is a ceiling the
 * platform enforces -- there is none for cost, which is why the sentence below
 * says so instead of implying one.
 */
const GENERATE_COST_LOW_USD = 0.003;
const GENERATE_COST_TYPICAL_USD = 0.006;
const GENERATE_COST_HIGH_USD = 0.03;
/**
 * The fourth number this component states, and until now the only one with no
 * machine behind it: how far back 最近沒有成功的生成 goes. The server decides it
 * (`generateFailureLimit`), the prose repeated it, and nothing compared the two.
 */
const GENERATE_FAILURE_LIMIT = 20; // one-number: generateFailureLimit

/**
 * 02:GEN-005's ceiling on the diagram upload — decoded bytes, enforced by
 * admission (`generateMaxDiagramBytes`). Checked here against `File.size`
 * directly: the browser holds the same bytes the server decodes, there is no
 * base64 step on this side of the wire that would change the count.
 */
const GENERATE_MAX_DIAGRAM_BYTES = 4000000; // one-number: generateMaxDiagramBytes
const GENERATE_DIAGRAM_TYPES = ["image/png", "image/jpeg", "image/webp"] as const;

/** 02:GEN-006's ceiling on reference Skills read as worked examples. */
const GENERATE_MAX_REFERENCES = 3; // one-number: generateMaxReferences

export function GenerateSkill({ initialTask = "" }: { initialTask?: string }) {
  const [task, setTask] = useState(initialTask);
  const [diagram, setDiagram] = useState<GenerateDiagram>();
  const [diagramName, setDiagramName] = useState("");
  const [diagramError, setDiagramError] = useState("");
  const [reading, setReading] = useState(false);
  const diagramFileRef = useRef<HTMLInputElement>(null);
  const [references, setReferences] = useState<{ id: string; name: string }[]>([]);
  const [rejected, setRejected] = useState<GenerateRejected>();
  const queryClient = useQueryClient();
  const mutation = useGenerateSkill();

  function handleDiagramChange(event: ChangeEvent<HTMLInputElement>) {
    const file = event.target.files?.[0];
    setDiagramError("");
    if (!file) return;
    if (!GENERATE_DIAGRAM_TYPES.includes(file.type as (typeof GENERATE_DIAGRAM_TYPES)[number])) {
      setDiagramError("圖片格式需為 PNG、JPEG 或 WebP。");
      event.target.value = "";
      return;
    }
    if (file.size > GENERATE_MAX_DIAGRAM_BYTES) {
      setDiagramError("圖片超過大小上限，請換一張較小的圖。");
      event.target.value = "";
      return;
    }
    const reader = new FileReader();
    reader.onload = () => {
      const result = String(reader.result);
      setDiagram({
        media_type: file.type as GenerateDiagram["media_type"],
        data: result.slice(result.indexOf(",") + 1),
      });
      setDiagramName(file.name);
      setReading(false);
    };
    reader.onerror = () => {
      setDiagramError("讀取圖片失敗，請重新選擇。");
      setReading(false);
    };
    setReading(true);
    reader.readAsDataURL(file);
  }

  function removeDiagram() {
    setDiagram(undefined);
    setDiagramName("");
    setDiagramError("");
    if (diagramFileRef.current) diagramFileRef.current.value = "";
  }

  function toggleReference(id: string, name: string) {
    setReferences((prev) => {
      if (prev.some((r) => r.id === id)) return prev.filter((r) => r.id !== id);
      if (prev.length >= GENERATE_MAX_REFERENCES) return prev;
      return [...prev, { id, name }];
    });
  }

  const submit = () => {
    setRejected(undefined);
    mutation.mutate(
      {
        task_description: task.trim() ? task : undefined,
        diagram,
        reference_skill_ids: references.length ? references.map((r) => r.id) : undefined,
      },
      {
        onSuccess: async () => {
          // The workspace list's key, not ["skills"]. The wrong key did two
          // things: left the list on /workspace/skills stale after a success,
          // and on Home matched ["skills","search",…] — re-running the search
          // and writing a second search_performed analytics event per success,
          // which is the funnel number the ⛔ boundary exists to protect.
          await queryClient.invalidateQueries({ queryKey: ["own-skills"] });
        },
        onSettled: async () => {
          // Both ways: a failure adds a row, and a success does not — but the
          // list is stale either way once a generation has been attempted.
          await queryClient.invalidateQueries({ queryKey: ["generate", "failures"] });
        },
        onError: (error) => {
          // The categorised 422 is the package's own findings, verbatim, exactly
          // as a failed import renders them (02:GEN-003). The uncategorised one is
          // a refusal around the model call — blank input, no allowance left,
          // output truncated — and has a sentence instead.
          setRejected(
            error instanceof ApiError && isCategorizedFindings(error.body)
              ? (error.body as GenerateRejected)
              : undefined,
          );
        },
      },
    );
  };

  return (
    <section>
      <h2>沒有夠接近的？讓平台依你的描述做一個</h2>
      <p className="note">
        平台會依你寫的任務描述產生一個 Skill 套件，放進你自己的工作區。 它
        <strong>不會進入公開目錄，也不會出現在搜尋結果裡</strong>——包括你自己搜尋的時候。
      </p>

      <label htmlFor="generate-task">任務描述</label>
      <textarea
        id="generate-task"
        rows={4}
        value={task}
        onChange={(e) => setTask(e.target.value)}
        placeholder="要完成什麼、輸入是什麼、預期產出是什麼。"
        disabled={mutation.isPending}
      />

      {/*
        02:GEN-005. `<input type="file">` shape copied from VersionUpload.tsx —
        this is the same "read a File, refuse it client-side before it ever
        reaches a request" pattern, just against an image type/size ceiling
        instead of a zip one.
      */}
      <label htmlFor="generate-diagram-file">流程圖或架構圖（選填）</label>
      <input
        id="generate-diagram-file"
        ref={diagramFileRef}
        type="file"
        accept="image/png,image/jpeg,image/webp"
        disabled={mutation.isPending}
        onChange={handleDiagramChange}
      />
      <p className="note">圖片會傳給模型參考，平台不會保留圖片本身，只留下它的雜湊。</p>
      {diagramError && <p role="alert">{diagramError}</p>}
      {diagram && (
        <p>
          已選擇 {diagramName}{" "}
          <button type="button" onClick={removeDiagram} disabled={mutation.isPending}>
            移除
          </button>
        </p>
      )}

      <ReferencePicker
        references={references}
        onToggle={toggleReference}
        disabled={mutation.isPending}
      />

      <dl>
        <dt>這一次最多會用到</dt>
        <dd>
          描述 {GENERATE_MAX_TASK_RUNES.toLocaleString("zh-TW")} 字、模型推理加輸出合計{" "}
          {GENERATE_MAX_OUTPUT_TOKENS.toLocaleString("zh-TW")} token、最多嘗試{" "}
          {GENERATE_MAX_ATTEMPTS} 次。
          <span className="note">
            {" "}
            這三個數字是伺服器實際擋你的上限，不是估計；超過第一個會被拒絕，超過第二個會直接停下、不重試。
          </span>
        </dd>
        <dt>預估成本</dt>
        <dd>
          約 US${GENERATE_COST_LOW_USD.toFixed(3)}–${GENERATE_COST_HIGH_USD.toFixed(2)}
          ，多數落在 US${GENERATE_COST_TYPICAL_USD.toFixed(3)} 上下
          <span className="note">
            {" "}
            ——估計值，非報價。來源：2026-08-25 對真實閘道生成 10 次的實付分布（最小 US$0.0038、中位
            US$0.0062、最大 US$0.0110，mini 級模型，皆為單次嘗試）。上緣按最多{" "}
            {GENERATE_MAX_ATTEMPTS} 次嘗試放寬並上取整，因為 10 次不是一個界。
            <strong>平台沒有為單次生成設定費用上限</strong>，所以這是估計不是保證。
            帶流程圖與帶參考各實測一次（US$0.0039、US$0.0040，2026-09-05），
            都落在區間內，但一次不是分布。
          </span>
        </dd>
      </dl>

      <button
        type="button"
        onClick={submit}
        disabled={mutation.isPending || reading || (task.trim() === "" && !diagram)}
      >
        {mutation.isPending ? "生成中…" : "生成一個 Skill"}
      </button>

      {mutation.isPending && <GenerateInFlight />}

      {/*
        A refusal the server did not categorise: blank description, no allowance
        left, or output past the token ceiling. The message is the whole answer,
        and each of those three already says what to do next.
      */}
      {mutation.error && !rejected && <p role="alert">生成失敗：{mutation.error.message}</p>}

      {rejected && <GenerateFailed rejected={rejected} onRetry={submit} />}

      {mutation.data && <GenerateSucceeded result={mutation.data} />}

      <GenerateHistory />
    </section>
  );
}

/**
 * 02:GEN-006: pick up to three Skills for the model to read as worked
 * examples. Two sources, same reason `useOwnSkills` is a second call rather
 * than a client-side filter of the search results: `useSkillSearch` is the
 * public catalogue (DISC-001, no session required) and does not include a
 * private, unpublished Skill the caller owns — the two lists answer different
 * questions about the same query text.
 *
 * The query is local state and debounced, not the URL: unlike Home's search
 * this picker has no shareable result page, only a selection that feeds one
 * submit.
 */
function ReferencePicker({
  references,
  onToggle,
  disabled,
}: {
  references: { id: string; name: string }[];
  onToggle: (id: string, name: string) => void;
  disabled: boolean;
}) {
  const [query, setQuery] = useState("");
  const [debounced, setDebounced] = useState("");
  useEffect(() => {
    const timer = setTimeout(() => setDebounced(query.trim()), 300);
    return () => clearTimeout(timer);
  }, [query]);

  const searching = debounced.length > 0;
  const search = useSkillSearch(debounced, {}, searching, "reference");
  const ownSkills = useOwnSkills();
  const ownMatches = searching
    ? (ownSkills.data?.skills ?? []).filter(
        (s) =>
          s.name.toLowerCase().includes(debounced.toLowerCase()) ||
          s.summary.toLowerCase().includes(debounced.toLowerCase()),
      )
    : [];

  const atLimit = references.length >= GENERATE_MAX_REFERENCES;
  const selectedIds = new Set(references.map((r) => r.id));

  return (
    <div>
      <label htmlFor="generate-reference-query">搜尋要參考的 Skill</label>
      <input
        id="generate-reference-query"
        type="text"
        value={query}
        onChange={(e) => setQuery(e.target.value)}
        placeholder="輸入名稱或關鍵字"
        disabled={disabled}
      />
      <p className="note">
        模型會把最多 {GENERATE_MAX_REFERENCES} 個你選的 Skill 的 SKILL.md
        當範例讀，產出仍是你工作區裡一個全新的 Skill。 有授權暫扣或禁止再散布的目錄 Skill
        無法被選為參考。
      </p>

      {references.length > 0 && (
        <ul className="badge-row" aria-label="已選的參考 Skill">
          {references.map((r) => (
            <li key={r.id}>
              <button type="button" onClick={() => onToggle(r.id, r.name)} disabled={disabled}>
                {r.name} ✕
              </button>
            </li>
          ))}
        </ul>
      )}
      {atLimit && (
        <p className="note" id="generate-reference-limit">
          已經選滿 {GENERATE_MAX_REFERENCES} 個，取消一個才能改選別的。
        </p>
      )}

      {searching && (
        <>
          <h3>搜尋結果</h3>
          {search.isFetching && <p className="note">搜尋中…</p>}
          <ul className="search-results">
            {(search.data?.results ?? []).map((hit) => (
              <ReferenceRow
                key={hit.skill_id}
                skillId={hit.skill_id}
                name={hit.name}
                summary={hit.summary}
                className="search-result"
                checked={selectedIds.has(hit.skill_id)}
                disabled={disabled || (!selectedIds.has(hit.skill_id) && atLimit)}
                onToggle={() => onToggle(hit.skill_id, hit.name)}
              />
            ))}
          </ul>
          <h3>我的 Skill</h3>
          <ul className="search-results">
            {ownMatches.map((s) => (
              <ReferenceRow
                key={s.skill_id}
                skillId={s.skill_id}
                name={s.name}
                summary={s.summary}
                className="download-item"
                checked={selectedIds.has(s.skill_id)}
                disabled={disabled || (!selectedIds.has(s.skill_id) && atLimit)}
                onToggle={() => onToggle(s.skill_id, s.name)}
              />
            ))}
          </ul>
        </>
      )}
    </div>
  );
}

function ReferenceRow({
  name,
  summary,
  className,
  checked,
  disabled,
  onToggle,
}: {
  skillId: string;
  name: string;
  summary: string;
  className: string;
  checked: boolean;
  disabled: boolean;
  onToggle: () => void;
}) {
  return (
    <li className={className}>
      <label>
        <input
          type="checkbox"
          checked={checked}
          disabled={disabled}
          aria-describedby={disabled && !checked ? "generate-reference-limit" : undefined}
          onChange={onToggle}
        />
        {name}
      </label>
      <p className="note">{summary}</p>
    </li>
  );
}

/**
 * GEN-003's read half: 「在工作區留下可查的失敗紀錄」.
 *
 * A separate clause from the verbatim findings shown above, which cover the
 * generation the user is looking at right now. This one has to outlive the
 * request — a failure the user walked away from is exactly the one they come
 * back to ask about.
 *
 * Absent when there is nothing to show, and that is not a §2.9 violation: an
 * empty history is a whole section with no rows, not a value position rendered
 * blank. A failed read IS a value position, and says so.
 */
function GenerateHistory() {
  const history = useGenerateFailures();

  if (history.isError) {
    return (
      <p className="note" role="status">
        過去的生成紀錄讀取失敗。這不影響你現在能不能生成。
      </p>
    );
  }
  const failures = history.data?.failures ?? [];
  if (failures.length === 0) return null;

  return (
    <details>
      <summary>最近沒有成功的生成（{failures.length} 次）</summary>
      <ul>
        {failures.map((f) => (
          <li key={f.occurred_at}>
            <Timestamp at={f.occurred_at} />
            {" — "}
            {failureSentence(f)}
          </li>
        ))}
      </ul>
      <p className="note">
        這些是沒有建立任何版本的那幾次，最多列最近 {GENERATE_FAILURE_LIMIT} 次。
        <strong>這裡沒有記下你當時輸入的任務描述</strong>
        ——那份文字跟著它產生的 Skill 走，刪掉 Skill 就跟著刪掉；這份紀錄保存得更久，
        兩邊各留一份等於一個沒有人做過的保存承諾。
      </p>
    </details>
  );
}

/**
 * Design system §2.12: an in-progress screen has to say which step it is on,
 * whether the step ends by itself, and **whether the user can leave**.
 *
 * The third answer is different from a Run's, and it is the reason this is not
 * the `InFlight` component. A Run is a River job consumed by cmd/worker and its
 * trace arrives by POST from the sandbox — the browser is in neither path, so
 * closing the tab is safe. A generation is one synchronous request: there is no
 * job, no run id, and an abandoned request is a cancelled generation. §2.2
 * forbids showing a promise nothing enforces, and "you can close this" would be
 * exactly that.
 */
function GenerateInFlight() {
  return (
    <div role="status" className="notice">
      <p>正在請模型寫這個 Skill，然後用與匯入完全相同的那道驗證檢查它。</p>
      <p>這一步會自己結束，通常十幾秒到一分鐘。</p>
      <p className="note">
        這一段沒有進度可以報——生成是一次呼叫，它要嘛回一個套件要嘛失敗， 沒有中間的量可以顯示。
      </p>
      <p>
        <strong>請不要關掉這個分頁</strong>
        ——這一次生成沒有背景工作可以接手，關掉就等於取消，而且不會留下任何半成品版本。
      </p>
    </div>
  );
}

/**
 * 02:GEN-003: the findings go to the user verbatim, not rewritten into
 * reassurance, and there are two named ways out. Nothing was stored — the
 * validation happens before the object store write and before the transaction —
 * so there is no half-made version to clean up, and saying so is part of the
 * answer.
 */
function GenerateFailed({
  rejected,
  onRetry,
}: {
  rejected: GenerateRejected;
  onRetry: () => void;
}) {
  return (
    <section role="alert">
      <h3>生成失敗：套件被擋下，沒有建立任何版本</h3>
      {/*
        Conditional, because the automatic retry does not always happen: a
        `possible-secret` finding is not retried (ADR-048), and saying "we
        already tried twice" about one attempt is the same §2.2 violation this
        file's in-flight block refuses to commit one component up. The server
        sends `attempts` on the 422 for exactly this sentence.
      */}
      <p className="note">
        {rejected.attempts > 1
          ? "平台已經自動用同一段描述再試過一次，第二次仍然沒有通過。下面是檢查逐字回報的內容，沒有經過改寫。"
          : "這一次沒有自動重試——被擋下的原因不是排版手滑，同一段描述再送一次會得到同樣的結果。下面是檢查逐字回報的內容，沒有經過改寫。"}
      </p>
      {/* h4：這一段的標題是上面那個 `h3 生成失敗`，而分組是它的內容。
          預設 3 對匯入頁是對的（那裡的父標題是 h2），對這裡不是。 */}
      <Findings findings={rejected} level={4} />
      <p>
        <button type="button" onClick={onRetry}>
          再試一次
        </button>{" "}
        或者改寫上面的任務描述再送出——把要做什麼、輸入是什麼、預期產出是什麼寫得更具體，通常比重試有用。
      </p>
    </section>
  );
}

/**
 * GEN-004: what the user must be told about a package nobody has looked at.
 *
 * The two absences come from GeneratedNotice rather than being written again
 * here. They were written again here, and the copy had already started to
 * drift: it said 「這份內容是模型剛剛寫出來的」 where the detail page says 「平台
 * 依任務描述生成的」, and it dropped the link to the first trial run. Findings.tsx
 * names the rule this broke — two copies is how one of the two screens quietly
 * rewrites a warning into reassurance.
 *
 * `attempts` stays local, because it belongs to this generation and not to the
 * package: 02:GEN-003 forbids the UI promising a success rate — one retry was
 * measured to move 80% to 90%, not to nothing — and "it took two goes" is the
 * honest form of that number.
 */
function GenerateSucceeded({
  result,
}: {
  result: { skill_id: string; version_number: number; attempts: number };
}) {
  return (
    <section role="status">
      <h3>已經產生一個 Skill，放在你的工作區</h3>
      <GeneratedNotice skillId={result.skill_id} />
      {result.attempts > 1 && (
        <p className="note">這一次生成試了 {result.attempts} 趟才通過驗證。</p>
      )}
      <p>
        <Link to="/skills/$skillId" params={{ skillId: result.skill_id }}>
          打開這個 Skill
        </Link>
      </p>
    </section>
  );
}
