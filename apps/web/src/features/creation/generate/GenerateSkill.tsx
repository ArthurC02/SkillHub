import { useRef, useState, type ChangeEvent } from "react";
import { ApiError } from "../../../core/api/client";
import { ReadFailure } from "../../../shared/ui/LoginRequired";
import { useGenerateAllowance, useGenerateSkill } from "../generate.service";
import { Timestamp } from "../../../shared/ui/Timestamp";
import {
  GENERATE_DIAGRAM_LIMIT,
  GENERATE_MAX_REFERENCES,
  generateDiagramProblem,
  generateFailureMessage,
  readGenerateDiagram,
} from "../generate.model";
import { isCategorizedFindings } from "../import.service";
import type { GenerateDiagram, GenerateRejected } from "../../../core/api/types";
import { GenerateHistory } from "./components/GenerateHistory";
import { GenerateInFlight } from "./components/GenerateInFlight";
import { GenerateFailed } from "./components/GenerateFailed";
import { GenerateSucceeded } from "./components/GenerateSucceeded";
import { ReferencePicker } from "../components/ReferencePicker";

const GENERATE_MAX_TASK_RUNES = 4000; // one-number: generateMaxTaskRunes

const GENERATE_MAX_OUTPUT_TOKENS = 16000; // one-number: generateMaxOutputTokens

const GENERATE_MAX_ATTEMPTS = 2; // one-number: generateMaxAttempts

const GENERATE_COST_LOW_USD = 0.003;

const GENERATE_COST_TYPICAL_USD = 0.006;

const GENERATE_COST_HIGH_USD = 0.03;

function useGenerateDiagram() {
  const [diagram, setDiagram] = useState<GenerateDiagram>();
  const [diagramName, setDiagramName] = useState("");
  const [diagramError, setDiagramError] = useState("");
  const [reading, setReading] = useState(false);
  const diagramFileRef = useRef<HTMLInputElement>(null);

  function handleDiagramChange(event: ChangeEvent<HTMLInputElement>) {
    const file = event.target.files?.[0];
    setDiagramError("");
    if (!file) return;
    setDiagram(undefined);
    setDiagramName("");
    const problem = generateDiagramProblem(file);
    if (problem) {
      setDiagramError(problem);
      event.target.value = "";
      return;
    }
    setReading(true);
    readGenerateDiagram(file).then(
      (read) => {
        setDiagram({
          media_type: read.media_type as GenerateDiagram["media_type"],
          data: read.data,
        });
        setDiagramName(file.name);
        setReading(false);
      },
      () => {
        setDiagramError("讀取圖片失敗，請重新選擇。");
        setReading(false);
        if (diagramFileRef.current) diagramFileRef.current.value = "";
      },
    );
  }

  function removeDiagram() {
    setDiagram(undefined);
    setDiagramName("");
    setDiagramError("");
    if (diagramFileRef.current) diagramFileRef.current.value = "";
  }

  return {
    diagram,
    diagramName,
    diagramError,
    reading,
    diagramFileRef,
    handleDiagramChange,
    removeDiagram,
  };
}

export function GenerateSkill({ initialTask = "" }: { initialTask?: string }) {
  const [task, setTask] = useState(initialTask);
  const {
    diagram,
    diagramName,
    diagramError,
    reading,
    diagramFileRef,
    handleDiagramChange,
    removeDiagram,
  } = useGenerateDiagram();
  const [references, setReferences] = useState<{ id: string; name: string }[]>([]);
  const mutation = useGenerateSkill();
  const rejected =
    mutation.error instanceof ApiError && isCategorizedFindings(mutation.error.body)
      ? (mutation.error.body as GenerateRejected)
      : undefined;

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
        平台會依你寫的任務描述產生一個小工具套件，放進你自己的工作區。 它
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
        <summary>附一張流程圖，或指定要參考的小工具（都是選填）</summary>

        <div className="field">
          <label htmlFor="generate-diagram-file">流程圖或架構圖（選填）</label>
          <input
            id="generate-diagram-file"
            ref={diagramFileRef}
            type="file"
            accept="image/png,image/jpeg,image/webp"
            aria-describedby={
              reading ? "generate-diagram-note generate-diagram-reading" : "generate-diagram-note"
            }
            disabled={mutation.isPending || reading}
            onChange={handleDiagramChange}
          />
        </div>
        <p className="note" id="generate-diagram-note">
          PNG、JPEG 或 WebP，最多 {GENERATE_DIAGRAM_LIMIT}。
          圖片會傳給模型參考，平台不會保留圖片本身，只留下一串無法還原成圖片的指紋。
        </p>
        {reading && (
          <p role="status" id="generate-diagram-reading">
            正在讀取流程圖…
          </p>
        )}
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

      <GenerateCostEstimate />

      {nothingToSend && (
        <p className="note" id="generate-why-disabled">
          還不能送出：任務描述與流程圖至少要有一個。
        </p>
      )}
      <button
        type="button"
        onClick={submit}
        aria-describedby={
          reading ? "generate-diagram-reading" : nothingToSend ? "generate-why-disabled" : undefined
        }
        disabled={mutation.isPending || reading || nothingToSend}
      >
        {mutation.isPending ? "生成中…" : "生成一個小工具"}
      </button>

      <GenerateOutcome mutation={mutation} rejected={rejected} onRetry={submit} />

      <GenerateHistory />
    </section>
  );
}

function GenerateOutcome({
  mutation,
  rejected,
  onRetry,
}: {
  mutation: ReturnType<typeof useGenerateSkill>;
  rejected?: GenerateRejected;
  onRetry: () => void;
}) {
  const failure =
    mutation.error instanceof ApiError
      ? generateFailureMessage(mutation.error.status, mutation.error.message)
      : generateFailureMessage(undefined, "");

  return (
    <>
      {mutation.isPending && <GenerateInFlight />}
      {mutation.error && !rejected && (
        <ReadFailure error={mutation.error} what="生成小工具">
          <p role="alert">{failure}</p>
        </ReadFailure>
      )}
      {rejected && <GenerateFailed rejected={rejected} onRetry={onRetry} />}
      {mutation.data && <GenerateSucceeded result={mutation.data} />}
    </>
  );
}

function GenerateAllowanceFacts() {
  const allowance = useGenerateAllowance();
  if (!allowance.data) return null;
  const { remaining_today, remaining_window, window_resets_at, limits } = allowance.data;
  return (
    <>
      <dt>免費生成額度</dt>
      <dd>
        今天還能生成 {remaining_today} 次（每天 {limits.daily} 次）；這 {limits.window_days}{" "}
        天內還有 {remaining_window} 次（共 {limits.window} 次），
        <Timestamp at={window_resets_at} /> 起恢復。
      </dd>
    </>
  );
}

function GenerateCostEstimate() {
  return (
    <>
      <dl>
        <GenerateAllowanceFacts />
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
    </>
  );
}
