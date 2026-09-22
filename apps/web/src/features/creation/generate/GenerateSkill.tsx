import { useEffect, useRef, useState, type ChangeEvent } from "react";
import { ApiError } from "../../../core/api/client";
import { useGenerateSkill } from "../generate.service";
import { isCategorizedFindings } from "../import.service";
import { useOwnSkills, useSkillSearch } from "../../skill";
import type { GenerateDiagram, GenerateRejected } from "../../../core/api/types";
import { GenerateHistory } from "./components/GenerateHistory";
import { GenerateInFlight } from "./components/GenerateInFlight";
import { GenerateFailed } from "./components/GenerateFailed";
import { GenerateSucceeded } from "./components/GenerateSucceeded";

const GENERATE_MAX_TASK_RUNES = 4000; // one-number: generateMaxTaskRunes

const GENERATE_MAX_OUTPUT_TOKENS = 16000; // one-number: generateMaxOutputTokens

const GENERATE_MAX_ATTEMPTS = 2; // one-number: generateMaxAttempts

const GENERATE_COST_LOW_USD = 0.003;

const GENERATE_COST_TYPICAL_USD = 0.006;

const GENERATE_COST_HIGH_USD = 0.03;

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
  const mutation = useGenerateSkill();
  const rejected =
    mutation.error instanceof ApiError && isCategorizedFindings(mutation.error.body)
      ? (mutation.error.body as GenerateRejected)
      : undefined;

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

  const submit = () =>
    mutation.mutate({
      task_description: task.trim() ? task : undefined,
      diagram,
      reference_skill_ids: references.length ? references.map((r) => r.id) : undefined,
    });

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
            disabled={mutation.isPending || reading}
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
