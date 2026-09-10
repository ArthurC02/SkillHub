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

const GENERATE_MAX_TASK_RUNES = 4000; // one-number: generateMaxTaskRunes
const GENERATE_MAX_OUTPUT_TOKENS = 16000; // one-number: generateMaxOutputTokens
const GENERATE_MAX_ATTEMPTS = 2; // one-number: generateMaxAttempts

const GENERATE_COST_LOW_USD = 0.003;
const GENERATE_COST_TYPICAL_USD = 0.006;
const GENERATE_COST_HIGH_USD = 0.03;
const GENERATE_FAILURE_LIMIT = 20; // one-number: generateFailureLimit

const GENERATE_MAX_DIAGRAM_BYTES = 4000000; // one-number: generateMaxDiagramBytes
const GENERATE_DIAGRAM_TYPES = ["image/png", "image/jpeg", "image/webp"] as const;

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

  const nothingToSend = task.trim() === "" && !diagram;
  const taskRunes = [...task].length;

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
          await queryClient.invalidateQueries({ queryKey: ["own-skills"] });
        },
        onSettled: async () => {
          await queryClient.invalidateQueries({ queryKey: ["generate", "failures"] });
        },
        onError: (error) => {
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
      <h2>讓平台依你的描述做一個</h2>
      <p className="note">
        平台會依你寫的任務描述產生一個 Skill 套件，放進你自己的工作區。 它
        <strong>不會進入公開目錄，也不會出現在搜尋結果裡</strong>——包括你自己搜尋的時候。
      </p>

      <div className="field">
        <label htmlFor="generate-task">任務描述</label>
        <textarea
          id="generate-task"
          rows={4}
          value={task}
          onChange={(e) => setTask(e.target.value)}
          placeholder="要完成什麼、輸入是什麼、預期產出是什麼。"
          aria-describedby="generate-task-count"
          disabled={mutation.isPending}
        />
        <p className="note field-count" id="generate-task-count">
          {taskRunes.toLocaleString("zh-TW")} / {GENERATE_MAX_TASK_RUNES.toLocaleString("zh-TW")} 字
          {taskRunes > GENERATE_MAX_TASK_RUNES && "——超過了，送出會被伺服器擋下"}
        </p>
      </div>

      <details>
        <summary>附一張流程圖，或指定要參考的 Skill（都是選填）</summary>

        <div className="field">
          <label htmlFor="generate-diagram-file">流程圖或架構圖（選填）</label>
          <input
            id="generate-diagram-file"
            ref={diagramFileRef}
            type="file"
            accept="image/png,image/jpeg,image/webp"
            aria-describedby="generate-diagram-note"
            disabled={mutation.isPending}
            onChange={handleDiagramChange}
          />
        </div>
        <p className="note" id="generate-diagram-note">
          PNG、JPEG 或 WebP，{GENERATE_MAX_DIAGRAM_BYTES / 1_000_000} MB 以內。
          圖片會傳給模型參考，平台不會保留圖片本身，只留下一串無法還原成圖片的指紋。
        </p>
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
      </details>

      <dl>
        <dt>預估成本</dt>
        <dd>
          約 US${GENERATE_COST_LOW_USD.toFixed(3)}–${GENERATE_COST_HIGH_USD.toFixed(2)}
          ，多數落在 US${GENERATE_COST_TYPICAL_USD.toFixed(3)} 上下——估計值，非報價。
        </dd>
      </dl>
      <details>
        <summary>這一次的上限，以及成本這個數字的來歷</summary>
        <dl>
          <dt>這一次最多會用到</dt>
          <dd>
            模型推理加輸出合計 {GENERATE_MAX_OUTPUT_TOKENS.toLocaleString("zh-TW")} token、最多嘗試{" "}
            {GENERATE_MAX_ATTEMPTS} 次。
          </dd>
        </dl>
        <p className="note">
          上限那三個數字是伺服器實際擋你的上限，不是估計；超過第一個會被拒絕，超過第二個會直接停下、不重試。
        </p>
        <p className="note">
          成本來源：2026-08-25 對真實閘道生成 10 次的實付分布（最小 US$0.0038、中位 US$0.0062、最大
          US$0.0110，mini 級模型，皆為單次嘗試）。上緣按最多 {GENERATE_MAX_ATTEMPTS}{" "}
          次嘗試放寬並上取整，因為 10 次不是一個界。
          <strong>平台沒有為單次生成設定費用上限</strong>，所以這是估計不是保證。
          帶流程圖與帶參考各實測一次（US$0.0039、US$0.0040，2026-09-05），
          都落在區間內，但一次不是分布。
        </p>
      </details>

      {nothingToSend && (
        <p className="note" id="generate-why-disabled">
          還不能送出：任務描述與流程圖至少要有一個。
        </p>
      )}
      <button
        type="button"
        onClick={submit}
        aria-describedby={nothingToSend ? "generate-why-disabled" : undefined}
        disabled={mutation.isPending || reading || nothingToSend}
      >
        {mutation.isPending ? "生成中…" : "生成一個 Skill"}
      </button>

      {mutation.isPending && <GenerateInFlight />}

      {mutation.error && !rejected && <p role="alert">生成失敗：{mutation.error.message}</p>}

      {rejected && <GenerateFailed rejected={rejected} onRetry={submit} />}

      {mutation.data && <GenerateSucceeded result={mutation.data} />}

      <GenerateHistory />
    </section>
  );
}

export function ReferencePicker({
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
      <div className="field">
        <label htmlFor="generate-reference-query">搜尋要參考的 Skill</label>
        <input
          id="generate-reference-query"
          type="text"
          value={query}
          onChange={(e) => setQuery(e.target.value)}
          placeholder="輸入名稱或關鍵字"
          disabled={disabled}
        />
      </div>
      <p className="note">
        模型會讀你選的 Skill 的說明檔（SKILL.md）當範例，最多 {GENERATE_MAX_REFERENCES} 個；
        產出仍是你工作區裡一個全新的 Skill。 有授權暫扣或禁止再散布的目錄 Skill 無法被選為參考。
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
      <p className="note">
        {rejected.attempts > 1
          ? "平台已經自動用同一段描述再試過一次，第二次仍然沒有通過。下面是檢查逐字回報的內容，沒有經過改寫。"
          : "這一次沒有自動重試——被擋下的原因不是排版手滑，同一段描述再送一次會得到同樣的結果。下面是檢查逐字回報的內容，沒有經過改寫。"}
      </p>
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
