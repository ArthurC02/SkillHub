import { Fragment, useEffect, useMemo, useRef, useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { Link } from "@tanstack/react-router";
import { ApiError } from "../api/client";
import {
  actOnCreationSession,
  createCreationSession,
  getCreationLimits,
  getCreationSession,
  listCreationSessions,
  streamCreationSession,
  type CreationAction,
  type CreationAttachment,
  type CreationSession as Session,
  type CreationSnapshot,
  type CreationState,
} from "../api/creation";
import { useCredits } from "../api/credits";
import type { CategorizedFindings, ImportFinding } from "../api/import";
import { useRuns } from "../api/runs";
import { TERMINAL_RUN_STATUSES } from "../api/trace";
import { ReadFailure } from "./LoginRequired";
import { ReferencePicker } from "./GenerateSkill";
import { Findings } from "./Findings";
import { ModelMarkdown } from "./ModelMarkdown";
import { Reveal } from "./Reveal";
import { Timestamp } from "./Timestamp";
import { runStatusLabel } from "../pages/RunEvaluation";
const labels: Record<CreationState, string> = {
  queued: "等待處理",
  working: "正在創作",
  waiting_input: "等待你的補充",
  waiting_confirmation: "等待你確認",
  draft_ready: "草稿可供檢查",
  candidate_ready: "候選版本已建立",
  saved: "已保存",
  cancelled: "已取消",
  failed: "這一步未完成",
  needs_reupload: "請重新上傳流程圖",
};
type Extra = Omit<CreationAction, "command_id" | "expected_revision" | "kind">;
/**
 * 這個限制在契約上是 `CreationAction.message` 的 `maxLength: 4000`，Go 再以 rune
 * 數檢一次。**這裡數的是 code point，也就是 Go 的 rune**——`.length` 會把一個 emoji
 * 數成 2，那是伺服器不會用的單位。
 */
const MAX_MESSAGE_RUNES = 4000;
/**
 * 一張圖能不能收，問這裡。
 *
 * 三個入口——挑檔案、貼上、拖進來——問的必須是同一個問題，而只有第一個能靠
 * `accept=""` 過濾：剪貼簿與拖放繞過它，所以一個 PDF 會安安靜靜地掛上去，等到送出
 * 才被拒絕。回傳的是那句話本身，因為三個入口都要當場說出來，而不是等 4xx。
 */
function diagramProblem(file: File): string | undefined {
  if (!["image/png", "image/jpeg", "image/webp"].includes(file.type))
    return "流程圖只收 PNG、JPEG 或 WebP。";
  const maxBytes = 4_000_000; // one-number: creationMaxDiagramBytes
  if (file.size === 0 || file.size > maxBytes)
    return "請選擇最多 4,000,000 位元組（約 3.8 MB）以內的流程圖。";
  return undefined;
}
function readImage(file: File): Promise<{ media_type: string; data: string }> {
  const problem = diagramProblem(file);
  if (problem) return Promise.reject(new Error(problem));
  return new Promise((resolve, reject) => {
    const reader = new FileReader();
    reader.onerror = () => reject(new Error("流程圖無法讀取，請重新選擇。"));
    reader.onload = () =>
      resolve({ media_type: file.type, data: String(reader.result).split(",")[1] });
    reader.readAsDataURL(file);
  });
}
/**
 * apps/platform's `skillpkg.Report` (creator/creation's `ValidateCreationDraft`
 * marshals it verbatim) is `{findings: Finding[], blocked, ...}` — a flat
 * array, not import's pre-grouped `{errors,warnings,infos}`. `Finding` itself
 * is field-for-field `ImportFinding` (severity/code/path/message/details), so
 * group by severity and reuse the SAME renderer failed imports use
 * (Findings.tsx) instead of a second component that flattens every finding
 * into identical bullets.
 */
function groupDraftFindings(raw: string): CategorizedFindings | undefined {
  try {
    const parsed = JSON.parse(raw) as { findings?: unknown };
    if (!Array.isArray(parsed.findings)) return undefined;
    const groups: CategorizedFindings = { errors: [], warnings: [], infos: [] };
    for (const item of parsed.findings as ImportFinding[]) {
      if (item.severity === "error") groups.errors.push(item);
      else if (item.severity === "warning") groups.warnings.push(item);
      else if (item.severity === "info") groups.infos.push(item);
    }
    return groups;
  } catch {
    return undefined;
  }
}
function DraftFindings({ raw }: { raw: string }) {
  const grouped = groupDraftFindings(raw);
  return grouped ? <Findings findings={grouped} level={4} /> : <p>{raw}</p>;
}
type RunObservation = {
  execution_status: string;
  evaluation?: {
    evaluation_available: boolean;
    overall?: string;
    criterion_results?: { result?: string }[];
  };
};
/** The newest `tool` message reporting on this run (creation.go's `attach_run`
 * appends one such message per confirmation; a later confirmation can attach
 * a newer run under the same candidate). */
function findRunObservation(
  messages: CreationSnapshot["messages"],
  runID: string,
): RunObservation | undefined {
  let found: RunObservation | undefined;
  for (const m of messages) {
    if (m.role !== "tool") continue;
    try {
      const parsed = JSON.parse(m.content) as RunObservation & { run_id?: string };
      if (parsed.run_id === runID) found = parsed;
    } catch {
      // Not every tool message is a run observation.
    }
  }
  return found;
}
type DiagramUnderstanding = {
  nodes: string[];
  conditions: string[];
  branches: string[];
  uncertainties: string[];
};
const diagramSections: (keyof DiagramUnderstanding)[] = [
  "nodes",
  "conditions",
  "branches",
  "uncertainties",
];
const diagramLabels: Record<keyof DiagramUnderstanding, string> = {
  nodes: "節點",
  conditions: "條件",
  branches: "分支",
  uncertainties: "不確定處",
};
function parseDiagramUnderstanding(raw: string): DiagramUnderstanding | undefined {
  try {
    const value: unknown = JSON.parse(raw);
    if (!value || typeof value !== "object" || Array.isArray(value)) return undefined;
    const record = value as Record<string, unknown>;
    if (Object.keys(record).length !== diagramSections.length) return undefined;
    if (!diagramSections.every((key) => Array.isArray(record[key]))) return undefined;
    const sections = Object.fromEntries(
      diagramSections.map((key) => [key, record[key] as unknown[]]),
    ) as Record<keyof DiagramUnderstanding, unknown[]>;
    if (sections.nodes.length === 0) return undefined;
    if (!diagramSections.every((key) => sections[key].every((item) => typeof item === "string")))
      return undefined;
    return Object.fromEntries(
      diagramSections.map((key) => [key, sections[key] as string[]]),
    ) as DiagramUnderstanding;
  } catch {
    return undefined;
  }
}
function DiagramUnderstandingView({ raw }: { raw: string }) {
  const structured = parseDiagramUnderstanding(raw);
  if (!structured)
    return (
      <>
        <p>{raw}</p>
        <p className="note">請在對話要求重新整理，或重新上傳後確認。</p>
      </>
    );
  return (
    <div>
      {diagramSections.map((key) => (
        <section key={key}>
          <h5>{diagramLabels[key]}</h5>
          {structured[key].length > 0 ? (
            <ul>
              {structured[key].map((item, i) => (
                <li key={i}>{item}</li>
              ))}
            </ul>
          ) : (
            <p>未列出</p>
          )}
        </section>
      ))}
    </div>
  );
}
/**
 * The pictures that belong to one turn.
 *
 * `thumbs` is the only place a picture can come from: the platform keeps the
 * digest and refuses the bytes (ADR-066 決策 4), so nothing serves it back and
 * `thumbs` holds only what THIS browser sent, for as long as this page lives.
 * After a reload — or on any other device — the turn says what was attached
 * instead of showing it. That sentence is not an error, so it is not an alert;
 * it is the same 「這裡沒有東西可以給你看，原因是這個」 the app says elsewhere.
 */
function Attachments({
  list,
  thumbs,
}: {
  list: CreationAttachment[];
  thumbs: Map<string, string>;
}) {
  return (
    <ul className="creation-attachments">
      {list.map((a) => {
        const url = thumbs.get(a.sha256);
        return (
          <li key={a.sha256} className="creation-attachment">
            {url ? (
              <img src={url} alt={"你在這一輪附上的流程圖（" + a.media_type + "）"} />
            ) : (
              <p className="note">平台不保存原圖，所以重新整理之後這裡只剩它的說明。</p>
            )}
            <span className="note">
              流程圖 · {a.media_type} · {a.bytes} 位元組
            </span>
          </li>
        );
      })}
    </ul>
  );
}
const CRITERION_RESULT_LABEL: Record<string, string> = {
  passed: "通過",
  failed: "不通過",
  undetermined: "無法判定",
};
type FetchObservation = {
  fetch: { url: string; status: string; bytes?: number; text?: string };
};
type RunToolObservation = {
  run_id: string;
  execution_status: string;
  evaluation?: {
    overall?: string;
    summary?: string;
    criterion_results?: { text?: string; result?: string; reason?: string }[];
  };
};
function parseObservation(raw: string): unknown {
  try {
    return JSON.parse(raw);
  } catch {
    return undefined;
  }
}
/**
 * 工具結果那一則訊息（`04` 丙-208）。
 *
 * 這些訊息有兩種：Go 寫給模型看的中文句子（「目錄搜尋需要關鍵字」之類），以及
 * **兩包 JSON**。JSON 那兩包在此之前是原樣倒進對話的——`attach_run` 的整包評估，
 * 還有 `fetch` 那一包**連同整個網頁的文字**。一個對話介面裡出現一整頁的 JSON，
 * 沒有人會讀它，而它把真正要讀的東西擠到看不見。
 *
 * 認得的就講成一句話，證據收進 `<details>`；認不得的原樣顯示——那些本來就是句子。
 *
 * **這裡只改呈現，不改信任**：`summary`、`reason` 是判定模型寫的字，而它寫的是受測
 * Skill 產生的輸出（`EvaluationText` 那條 LLM01 通道的同一批文字），`fetch.text` 是
 * 抓回來的網頁。三者都是不受信任的文字，所以它們**只能是文字**——React 會轉義，這裡
 * 沒有任何一處把它們當成標記（`04` 丙-206 要裁的正是那件事）。
 */
function ToolObservation({ raw }: { raw: string }) {
  const parsed = parseObservation(raw);
  if (parsed && typeof parsed === "object") {
    const asFetch = parsed as Partial<FetchObservation>;
    if (asFetch.fetch && typeof asFetch.fetch.url === "string") {
      const f = asFetch.fetch;
      return (
        <>
          <span className="creation-text">
            讀取網頁 {f.url}：{FETCH_STATUS_LABEL[f.status] ?? f.status}
            {f.bytes !== undefined && `（${f.bytes} 位元組）`}
          </span>
          {!!f.text && (
            <details>
              <summary>讀到的網頁內容（{[...f.text].length} 字）</summary>
              <pre className="skill-md">
                <Reveal text={f.text} />
              </pre>
            </details>
          )}
        </>
      );
    }
    const asRun = parsed as Partial<RunToolObservation>;
    if (typeof asRun.run_id === "string" && typeof asRun.execution_status === "string") {
      const results = asRun.evaluation?.criterion_results ?? [];
      const count = (r: string) => results.filter((x) => x.result === r).length;
      const overall = asRun.evaluation?.overall ?? "";
      return (
        <>
          <span className="creation-text">
            試跑結果：{runStatusLabel(asRun.execution_status)}；評估：
            {ROUND_OVERALL_LABEL[overall] ?? "無評估"}
            {results.length > 0 &&
              `（通過 ${count("passed")}／不通過 ${count("failed")}／無法判定 ${count("undetermined")}）`}
          </span>
          {!!asRun.evaluation?.summary && (
            <span className="creation-text">{asRun.evaluation.summary}</span>
          )}
          {/* 逐條判定**不收進 `<details>`**：設計 §2.10 第 7 項把「任務判定」列在
              永不折疊的封閉清單裡，而每一條驗收條件的 passed／failed／undetermined
              就是判定。這裡也不需要折——這則訊息原本的問題是一整包 JSON 和一整頁
              網頁文字，不是這幾行；一次試跑的條件是個位數。 */}
          {results.length > 0 && (
            <ul>
              {results.map((c, i) => (
                <li key={i}>
                  {CRITERION_RESULT_LABEL[c.result ?? ""] ?? c.result ?? "無結果"}：{c.text}
                  {!!c.reason && `——${c.reason}`}
                </li>
              ))}
            </ul>
          )}
        </>
      );
    }
  }
  return <span className="creation-text">{raw}</span>;
}
/**
 * 這一步在做什麼（`04` 丙-205）。
 *
 * 在此之前等待中的畫面只說「正在創作」四個字，而那四個字要涵蓋五種完全不同的步驟。
 * **這不需要任何後端改動**：每一種步驟要做什麼，都是它進 `working` 之前那份快照
 * 已經決定好的事——所以這裡是推出來的，不是回報回來的。
 *
 * 順序就是 Go 的順序：連網在模型呼叫之前（`confirm_fetch` 把網址留在快照上，由
 * Worker 在呼叫前抓）；一張新圖會把 `diagram_understanding` 與 `brief_confirmed`
 * 一起清掉，所以圖那一條要排在需求之前。
 */
function stepDescription(p: CreationSnapshot): string {
  if (p.pending_fetch_url) return "正在讀你同意的那個網頁，讀完再繼續。";
  if (p.diagram_fingerprint && !p.diagram_understanding) return "正在讀你附上的流程圖。";
  if (!p.brief_confirmed) return "正在整理需求與驗收條件。";
  if (!p.draft) return "正在寫第一份草稿。";
  if (p.run_unmet) return "正在依試跑結果修訂草稿。";
  return "正在修訂草稿。";
}
function declaredReferenceField(value?: string) {
  return value?.trim() ? value : "未宣告";
}
const TIER_LABEL: Record<string, string> = { curated: "精選", indexed: "已索引" };
function referenceTierLabel(tier?: string): string {
  return TIER_LABEL[tier ?? ""] ?? "不在目錄";
}
function referenceScanLabel(scanStatus?: string, warnings?: number): string {
  if (scanStatus === "scanned")
    return (warnings ?? 0) > 0 ? `已掃描，${warnings} 個警告` : "已掃描，無警告";
  if (scanStatus === "unavailable") return "沒有掃描紀錄";
  return "未知";
}
const FETCH_STATUS_LABEL: Record<string, string> = {
  ok: "已讀取",
  blocked: "被拒絕或被網路環境擋住（不重試）",
  not_found: "頁面不存在",
  unsupported: "不是文字頁面",
  network_error: "網路錯誤（重試一次仍失敗）",
  declined: "使用者不同意",
};
const ROUND_OVERALL_LABEL: Record<string, string> = {
  met: "達成",
  partially_met: "部分達成",
  not_met: "未達成",
};
function truncateForTimeline(s: string, n = 120): string {
  return s.length > n ? s.slice(0, n) + "…" : s;
}
type TimelineItem = { key: string; text: string };
/** 每輪的 Run、評估、建議、回饋串成時間線 — everything it needs is already in
 * `p.messages` (creation.go appends one `tool` message per attach_run, one
 * `tool` message per fetch, and the model's own assistant turns); this just
 * walks that array once and keeps the events a person would call "a round". */
function buildRoundTimeline(messages: CreationSnapshot["messages"]): TimelineItem[] {
  const items: TimelineItem[] = [];
  let round = 0;
  let afterQuestion = false;
  let afterTrialAnchor = false;
  messages.forEach((m, i) => {
    let setQuestion = false;
    let setTrialAnchor = false;
    if (m.role === "tool" && m.content.startsWith('{"evaluation"')) {
      try {
        const parsed = JSON.parse(m.content) as RunObservation;
        if (parsed.evaluation) {
          round += 1;
          const overall = parsed.evaluation.overall ?? "";
          const results = parsed.evaluation.criterion_results ?? [];
          const count = (r: string) => results.filter((x) => x.result === r).length;
          items.push({
            key: `t-${i}`,
            text: `第 ${round} 次試跑：${ROUND_OVERALL_LABEL[overall] ?? overall}（通過 ${count("passed")}／不通過 ${count("failed")}／無法判定 ${count("undetermined")}）`,
          });
          setTrialAnchor = true;
        }
      } catch {
        // Not a parseable evaluation observation; skip it.
      }
    } else if (m.role === "tool" && m.content.startsWith('{"fetch"')) {
      try {
        const parsed = JSON.parse(m.content) as { fetch: { url: string; status: string } };
        items.push({
          key: `t-${i}`,
          text: `讀取網頁：${parsed.fetch.url}（${FETCH_STATUS_LABEL[parsed.fetch.status] ?? parsed.fetch.status}）`,
        });
      } catch {
        // Not a parseable fetch observation; skip it.
      }
    } else if (m.role === "assistant" && m.content.startsWith("這次試跑有條件沒過")) {
      items.push({ key: `t-${i}`, text: `系統問你：${m.content.split("\n")[0]}` });
      setQuestion = true;
    } else if (m.role === "user" && afterQuestion) {
      items.push({ key: `t-${i}`, text: `你回答：${truncateForTimeline(m.content)}` });
      setTrialAnchor = true;
    } else if (m.role === "assistant" && afterTrialAnchor) {
      items.push({ key: `t-${i}`, text: `模型建議：${m.content.slice(0, 120)}` });
    }
    afterQuestion = setQuestion;
    afterTrialAnchor = setTrialAnchor;
  });
  return items;
}
export function CreationSession() {
  const client = useQueryClient();
  const [id, setID] = useState(""),
    [picking, setPicking] = useState(false),
    [dragging, setDragging] = useState(false),
    [message, setMessage] = useState(""),
    [budget, setBudget] = useState(""),
    [file, setFile] = useState<File>(),
    [refs, setRefs] = useState<{ id: string; name: string }[]>([]),
    [raiseBudget, setRaiseBudget] = useState(""),
    [raiseError, setRaiseError] = useState(""),
    [error, setError] = useState<unknown>(),
    [busy, setBusy] = useState(false);
  /**
   * `<input type="file">` is uncontrolled: `setFile(undefined)` empties React's
   * copy and leaves the DOM's `value` holding the same path, so choosing THE
   * SAME file again fires no `change` and the pick silently does nothing. Every
   * place that drops the attachment has to clear both. `GenerateSkill.tsx` has
   * had this pair since it shipped; this screen was missing it.
   */
  const fileInput = useRef<HTMLInputElement>(null);
  const clearFile = () => {
    setFile(undefined);
    if (fileInput.current) fileInput.current.value = "";
  };
  /**
   * The pictures this page has in its hands, by digest.
   *
   * A sent picture is never served back (ADR-066 決策 4 keeps the digest and
   * refuses the bytes), so the only way a conversation can show one is for the
   * browser that sent it to keep holding the `File`. That is what this is: an
   * object URL per digest, alive for this page and revoked when it unmounts.
   * The digest is the key because the server hands it back on the attachment it
   * just recorded, which is how the two halves find each other.
   */
  /**
   * 挑檔案、貼上、拖進來，三個入口都走這裡：當場檢查、當場說出不能收的理由，而不是
   * 讓一個 PDF 掛在輸入區上等到送出才被拒絕。被拒絕的那一次連 DOM 的 value 一起清，
   * 否則下一次選同一個檔案不會觸發 `change`（與 `clearFile` 同一個理由）。
   */
  const chooseFile = (picked?: File) => {
    if (!picked) return;
    const problem = diagramProblem(picked);
    if (problem) {
      clearFile();
      setError(new Error(problem));
      return;
    }
    setError(undefined);
    setFile(picked);
  };
  const thumbs = useRef(new Map<string, string>());
  useEffect(() => {
    const held = thumbs.current;
    return () => held.forEach((url) => URL.revokeObjectURL(url));
  }, []);
  // The picture that is in the composer but not sent yet. ChatGPT shows it, and
  // the reason is not decoration: it is the only confirmation that the file the
  // person picked is the file they meant.
  const preview = useMemo(() => (file ? URL.createObjectURL(file) : undefined), [file]);
  useEffect(
    () => () => {
      if (preview) URL.revokeObjectURL(preview);
    },
    [preview],
  );
  const pending = useRef<{ key: string; body: CreationAction } | undefined>(undefined);
  const startPending = useRef<
    { key: string; body: { id: string; message: string; budget_usd: number } } | undefined
  >(undefined);
  const sessions = useQuery({
    queryKey: ["creation-sessions"],
    queryFn: listCreationSessions,
    retry: false,
  });
  const limits = useQuery({
    queryKey: ["creation-limits"],
    queryFn: getCreationLimits,
    retry: false,
  });
  // ADR-069 / `05` R-71: the step stream stands the poll down, and only while
  // it is actually delivering. `streaming` is set by the connection itself
  // (open/error), never assumed — every way an SSE connection fails is silent,
  // so the poll below is the floor and the stream is the improvement on top.
  const [streaming, setStreaming] = useState(false);
  const current = useQuery({
    queryKey: ["creation-session", id],
    queryFn: () => getCreationSession(id),
    enabled: !!id,
    retry: false,
    refetchInterval: (q) =>
      !streaming && ["queued", "working"].includes(q.state.data?.state ?? "") ? 1000 : false,
  });
  useEffect(() => {
    if (!id) return;
    // Deliberately not gated on the current state: a session that is
    // `waiting_input` when this mounts becomes `queued` the moment the person
    // sends something, and a stream opened only for the busy states would miss
    // exactly the transition it exists to deliver. The server closes the
    // connection itself once the session is terminal (creation_stream.go), so
    // the ending is its decision, not a guess made here.
    return streamCreationSession(
      id,
      (s) => client.setQueryData(["creation-session", id], s),
      setStreaming,
    );
  }, [id, client]);
  const session = current.data,
    p = session?.snapshot;
  // CRED-001 (ADR-068)'s gate ① applies only to STARTING a new session — an
  // already-running one already reserved its budget, so `!session` gates it
  // the same way the budget input a few lines below is only asked once.
  // `credits.data` is undefined while loading and on every deployment today
  // (the route is not mounted yet, see api/credits.ts), which must read as
  // "nothing to show", never as blocked (04 乙-2's rule, applied here too).
  const credits = useCredits();
  const creditsBlocked = !session && !!credits.data && !credits.data.can_start;
  const runs = useRuns(p?.candidate?.test_case_id, Boolean(p?.candidate?.test_case_id));
  const latest = runs.data?.pages[0]?.runs.find((r) => TERMINAL_RUN_STATUSES.has(r.status));
  const run = p?.candidate?.run_id ? findRunObservation(p.messages, p.candidate.run_id) : undefined;
  const roundTimeline = p ? buildRoundTimeline(p.messages) : [];
  const runNotPassing =
    !!run && (run.execution_status !== "succeeded" || run.evaluation?.overall !== "met");
  const terminal = !!session && ["saved", "cancelled"].includes(session.state);
  const working = !!session && ["queued", "working"].includes(session.state);
  const locked = busy || working || terminal;
  const save = (value: Session) => {
    setID(value.id);
    client.setQueryData<Session>(["creation-session", value.id], (old) =>
      old && old.revision > value.revision ? old : value,
    );
    void client.invalidateQueries({ queryKey: ["creation-sessions"] });
  };
  const send = async (value: Session, kind: CreationAction["kind"], extra: Extra = {}) => {
    const key = JSON.stringify([value.id, kind, extra]);
    if (pending.current?.key !== key)
      pending.current = {
        key,
        body: {
          command_id: crypto.randomUUID(),
          expected_revision: value.revision,
          kind,
          ...extra,
        },
      };
    try {
      const next = await actOnCreationSession(value.id, pending.current.body);
      pending.current = undefined;
      save(next);
      return next;
    } catch (err) {
      if (err instanceof ApiError && err.status === 409) {
        pending.current = undefined;
        await client.invalidateQueries({ queryKey: ["creation-session", value.id] });
      }
      throw err;
    }
  };
  const perform = async (kind: CreationAction["kind"], extra: Extra = {}) => {
    if (!session) return;
    setBusy(true);
    setError(undefined);
    try {
      await send(session, kind, extra);
      if (kind === "message") setMessage("");
    } catch (err) {
      setError(err);
    } finally {
      setBusy(false);
    }
  };
  const submitRaiseBudget = async () => {
    if (!p || !limits.data) return;
    const amount = Number(raiseBudget);
    if (!Number.isFinite(amount) || amount <= p.budget_usd || amount > limits.data.max_budget_usd) {
      setRaiseError(
        `請填寫高於目前上限 $${p.budget_usd} 且不超過 $${limits.data.max_budget_usd} 的金額。`,
      );
      return;
    }
    setRaiseError("");
    await perform("raise_budget", { budget_usd: amount });
  };
  /**
   * 讓最新的一則留在視線裡——**但只在你本來就在底下的時候**。
   *
   * 這是聊天介面的通則（見 2026-09-09 那批的來源）：捲上去看前面幾輪的人，不該被
   * 新到的訊息拉回底部。所以捲動時記下「現在算不算在底下」，而只有那個答案是「算」
   * 的時候，訊息數變多才把錨點捲進來。200px 是那個「算在底下」的寬容值。
   *
   * `scrollIntoView?.()` 的問號不是防衛式寫法：jsdom 沒有實作它，而這個元件在
   * jsdom 裡被測。
   */
  const bottom = useRef<HTMLDivElement>(null);
  const atBottom = useRef(true);
  useEffect(() => {
    const onScroll = () => {
      atBottom.current =
        window.innerHeight + window.scrollY >= document.documentElement.scrollHeight - 200;
    };
    window.addEventListener("scroll", onScroll, { passive: true });
    return () => window.removeEventListener("scroll", onScroll);
  }, []);
  const messageCount = p?.messages.length ?? 0;
  useEffect(() => {
    if (messageCount > 0 && atBottom.current)
      bottom.current?.scrollIntoView?.({ block: "nearest" });
  }, [messageCount]);
  const submit = async () => {
    setBusy(true);
    setError(undefined);
    try {
      /*
       * 素材種類由輸入區的內容推出來，不再由一組 radio 先選（2026-09-08）。
       *
       * ── 稍晚同日：文字不再算「另一種素材」 ──────────────────────────────
       * 這裡本來擋下「圖＋文字」並叫人分兩次送。那不是版面選擇，是後端當時的形狀：
       * `diagram` 與 `select_references` 兩個 action 不收 `message`。**現在收了**
       * （creation.go 的 `attachNote`），所以圖或參考可以帶著那句話一起送——而那正
       * 是人本來就會做的事：「這是我的流程，我想把它變成一個 Skill」。
       *
       * 還擋著的只剩「圖＋參考」：那真的是兩個 kind，一次呼叫帶不了兩個，而送完一輪
       * 就進 working、下一個 action 要等新的 revision。所以這裡擋下來並說出順序，而
       * 不是連送兩次然後第二次撞 409。
       */
      const note = message.trim();
      const mode = file ? "diagram" : refs.length > 0 ? "references" : "message";
      if (!file && refs.length === 0 && !note)
        throw new Error("請描述想完成的任務，或附一張流程圖，或挑一個要參考的 Skill。");
      // 說出超過多少，而不是讓契約的 maxLength 回一句泛用的 400。**擋在這裡不等於
      // 這裡是強制者**：Go 與契約各檢一次，這一句只是把同一個上限講成人話。
      if ([...note].length > MAX_MESSAGE_RUNES)
        throw new Error(
          `文字說明最多 ${MAX_MESSAGE_RUNES} 字，目前 ${[...note].length} 字，請先剪短。`,
        );
      if (file && refs.length > 0)
        throw new Error(
          "流程圖和參考 Skill 一次只能送一種。先送其中一種，Agent 讀完之後再送另一種；文字說明可以跟著任一種一起送。",
        );
      const diagram = mode === "diagram" ? (file ? await readImage(file) : undefined) : undefined;
      if (mode === "diagram" && !diagram) throw new Error("請先選擇流程圖。");
      let value = session;
      if (!value) {
        const amount = Number(budget);
        if (!Number.isFinite(amount) || amount <= 0)
          throw new Error("請填寫這次同意支付的美元預算上限。");
        if (
          limits.data &&
          (amount < limits.data.min_budget_usd || amount > limits.data.max_budget_usd)
        )
          throw new Error(
            `請填寫介於 $${limits.data.min_budget_usd} 與 $${limits.data.max_budget_usd} 之間的預算上限。`,
          );
        const initial = mode === "message" ? message : "";
        const key = JSON.stringify([initial, amount]);
        if (startPending.current?.key !== key)
          startPending.current = {
            key,
            body: { id: crypto.randomUUID(), message: initial, budget_usd: amount },
          };
        value = await createCreationSession(startPending.current.body);
        save(value);
        startPending.current = undefined;
        if (mode === "message") {
          setMessage("");
          return;
        }
      }
      if (mode === "message") {
        await send(value, "message", { message });
        setMessage("");
      }
      // 送出成功之後，這一輪放進輸入區的東西全部清掉——三種素材都是。
      // 參考 Skill 本來沒有清：留下來的 chip 會被下一次的守門讀成「你又挑了參考」，
      // 於是你想補一句話卻被擋，而錯誤訊息叫你去做你剛剛做完的事。
      if (mode === "diagram") {
        const next = await send(value, "diagram", { diagram, ...(note ? { message: note } : {}) });
        // Hold on to the picture we just sent, keyed by the digest the server
        // recorded for it: that is the join between the turn in the history and
        // the only copy of the image that exists on this side.
        const recorded = next.snapshot.attachments ?? [];
        const mine = recorded[recorded.length - 1];
        if (mine && file) thumbs.current.set(mine.sha256, URL.createObjectURL(file));
        clearFile();
        setMessage("");
      }
      if (mode === "references") {
        await send(value, "select_references", {
          reference_skill_ids: refs.map((r) => r.id),
          ...(note ? { message: note } : {}),
        });
        setRefs([]);
        setMessage("");
      }
    } catch (err) {
      setError(err);
    } finally {
      setBusy(false);
    }
  };
  return (
    <div>
      <h3>和 Agent 一起創作 Skill</h3>
      <p className="note">
        逐步確認需求與草稿，保存到私人工作區。模型處理與改善會使用這次核准的預算。
      </p>
      <ReadFailure error={sessions.error ?? current.error} what="創作紀錄" />
      {sessions.data && sessions.data.length > 0 && (
        <label>
          恢復創作
          <select
            aria-label="恢復創作"
            value={id}
            disabled={busy}
            onChange={(e) => {
              setID(e.target.value);
              setMessage("");
              clearFile();
              setRefs([]);
              pending.current = undefined;
              setError(undefined);
            }}
          >
            <option value="">開始新的創作</option>
            {sessions.data.slice(0, 50).map((s) => (
              <option key={s.id} value={s.id}>
                {s.snapshot.brief.slice(0, 40) || "尚未確認需求"} · {labels[s.state]}
              </option>
            ))}
          </select>
        </label>
      )}
      {/* 這一行本來就是 `role="status"`——一個即時區域，狀態一變就會被念出來。
          等待中多說一句「這一步在做什麼」，念出來的因此是有內容的一句話，而不是
          「正在創作」四個字對五種步驟講同一遍。不新增元素，也就不多一個即時區域。 */}
      {session && (
        <p role="status">
          創作狀態：{labels[session.state]}
          {working && p && `——${stepDescription(p)}`}
        </p>
      )}
      {session && !terminal && (
        <p>
          這次創作可進行到 <Timestamp at={session.deadline} />
          ；紀錄保留到 <Timestamp at={session.expires_at} />。
        </p>
      )}
      {working && (
        <p className="note">
          可以關掉這一頁；回來時從「恢復創作」繼續。上次更新{" "}
          {/* `current` polls every 1s while queued/working (refetchInterval
              above), the same cadence InFlight.tsx uses to justify its own
              `relative` Timestamp — see InFlight.tsx. */}
          <Timestamp at={session.updated_at} relative />
        </p>
      )}
      {/* 停止這一步，而不是整場（`04` 丙-203）。`disabled={busy}` 而不是 `locked`：
          `locked` 把 working 也算進去，而這顆按鈕存在的理由就是 working。
          刻意不是 `.action`——停止不是這一頁要人做的那件事。 */}
      {working && (
        <button type="button" disabled={busy} onClick={() => void perform("stop_step")}>
          停止這一步
        </button>
      )}
      {p ? (
        <>
          <p>
            預算上限 $ {p.budget_usd}；已知費用{" "}
            {p.spent_usd === undefined ? "未知" : "$ " + p.spent_usd}；仍占用預算 $ {p.reserved_usd}
            。{p.usage_unknown && "部分用量未能取得，費用仍是未知，不能當作零。"}
          </p>
          {limits.data && (
            <p>
              已用 {p.steps}／{limits.data.max_steps} 步、工具 {p.tool_calls}／
              {limits.data.max_tool_calls} 次
            </p>
          )}
          {limits.data &&
            (session?.state === "failed" ||
              p.budget_usd - (p.spent_usd ?? 0) - p.reserved_usd <
                2 * limits.data.min_budget_usd) && (
              <div>
                <label>
                  提高這次預算上限（美元）
                  <input
                    aria-label="提高這次預算上限（美元）"
                    inputMode="decimal"
                    disabled={busy}
                    value={raiseBudget}
                    onChange={(e) => setRaiseBudget(e.target.value)}
                  />
                </label>
                <button type="button" disabled={busy} onClick={() => void submitRaiseBudget()}>
                  提高預算後繼續
                </button>
                {!!raiseError && <p role="alert">{raiseError}</p>}
              </div>
            )}
        </>
      ) : (
        <>
          {/*
            CRED-001. `credits.data` is undefined on every deployment today
            (route not mounted yet — see api/credits.ts), so this whole block
            renders nothing until that lands: no ceiling is shown before one
            is enforced (04 乙-2).
          */}
          {credits.data &&
            (creditsBlocked ? (
              <p className="note" id="creation-credits-why-disabled">
                {credits.data.block_reason}
              </p>
            ) : (
              <p className="note">
                目前餘額 {credits.data.balance_credits} 點；這一場大約要{" "}
                {credits.data.estimated_session.low_credits}–
                {credits.data.estimated_session.high_credits} 點
                {credits.data.estimated_session.estimated && "（樣本不足，此為估計值）"}。
              </p>
            ))}
          <label>
            這次預算上限（美元）
            {limits.data && (
              <span className="note">
                （介於 $ {limits.data.min_budget_usd} 與 $ {limits.data.max_budget_usd} 之間）
              </span>
            )}
            <input
              aria-label="這次預算上限（美元）"
              inputMode="decimal"
              value={budget}
              onChange={(e) => setBudget(e.target.value)}
            />
          </label>
        </>
      )}
      {!!error && (
        <ReadFailure error={error} what="互動創作">
          <p role="alert">
            {error instanceof ApiError && error.status === 409
              ? "進度已更新，輸入仍保留。請檢查最新內容後再送出。"
              : error instanceof TypeError
                ? "網路連線失敗，請重試。"
                : error instanceof Error
                  ? error.message
                  : "這一步未完成，請重試。"}
          </p>
        </ReadFailure>
      )}
      {p && (
        <>
          {/* 2026-09-08：這裡本來是一個編號 `<ol>`，每一列前面掛「你：」。多輪的
              流程一直都在，但**對話這個介面從來沒有被畫過**。角色從行內粗體變成
              訊息上方的標籤，列變成訊息塊，樣式全在 `index.css` 的 `.creation-log`
              （沒有新 token、沒有新字級）。編號拿掉了：對話不是編號清單。 */}
          {/* ── 2026-09-09：圖片進入對話 ────────────────────────────────
              在這之前圖片只在畫面別處留下一句「已附上流程圖」：**你送出去的東西，
              對話裡看不到**。現在它坐在它所屬的那一輪裡——打了字就在你那則訊息
              下面，沒打字就自成一塊，位置由 `message_index` 決定而不是由這裡猜。
              第二次上傳不再蓋掉第一次：`attachments` 是清單，`diagram_*` 三個
              欄位仍然是「最新那一張」給模型與 materialize 用。 */}
          {/* `role="log"` 是聊天視窗的那個角色：它隱含 `aria-live="polite"`，所以
              新到的一則會被念出來、而且是排隊念不是打斷。**掛在外面的 `<div>` 而不是
              `<ol>` 上**：角色會取代元素本來的語意，掛在清單上會讓底下的 `<li>` 變成
              沒有清單的清單項。名字是必要的——有名字的即時區域，螢幕閱讀器會先說出
              它是哪一區。 */}
          <div role="log" aria-label="與 Agent 的對話">
            <ol className="creation-log">
              {p.messages.map((m, i) => {
                const here = (p.attachments ?? []).filter((a) => a.message_index === i);
                return (
                  <Fragment key={i}>
                    {/* 沒有文字的上傳落在「下一則訊息」的索引上，所以那一則不是你的
                      話時，圖自己是一塊——它確實發生在這兩輪之間。 */}
                    {m.role !== "user" && here.length > 0 && (
                      <li data-role="user">
                        <span className="creation-who">你</span>
                        <Attachments list={here} thumbs={thumbs.current} />
                      </li>
                    )}
                    <li data-role={m.role}>
                      <span className="creation-who">
                        {{ user: "你", assistant: "Agent", tool: "工具結果" }[m.role]}
                      </span>
                      {/* 三種角色三種算繪，而分界是信任而不是外觀（`05` R-70，
                          2026-09-09 簽署）。`assistant` 得到白名單裡的標記；
                          `user` 是自己打的字，維持純文字；`tool` 走
                          ToolObservation，**而且它裡面的字一律是文字**——`fetch`
                          那種訊息裝的是抓回來的整頁網頁，是攻擊者直接寫的，不必
                          先騙過模型，所以它是這三種裡最不可信的一種。
                          換行仍然是內容的一部分（`04` 丙-207）：兩條路徑都靠
                          `white-space: pre-wrap` 留住它。 */}
                      {m.role === "tool" ? (
                        <ToolObservation raw={m.content} />
                      ) : m.role === "assistant" ? (
                        <ModelMarkdown text={m.content} />
                      ) : (
                        <span className="creation-text">{m.content}</span>
                      )}
                      {m.role === "user" && here.length > 0 && (
                        <Attachments list={here} thumbs={thumbs.current} />
                      )}
                    </li>
                  </Fragment>
                );
              })}
              {/* 剛送出、模型還沒回話的那一張。 */}
              {(p.attachments ?? []).some((a) => a.message_index >= p.messages.length) && (
                <li data-role="user">
                  <span className="creation-who">你</span>
                  <Attachments
                    list={(p.attachments ?? []).filter((a) => a.message_index >= p.messages.length)}
                    thumbs={thumbs.current}
                  />
                </li>
              )}
            </ol>
            {/* 捲動的錨點：新的一則到了，如果你本來就在底下，就把這裡捲進視線。 */}
            <div ref={bottom} />
          </div>
          {roundTimeline.length > 0 && (
            <section>
              <h4>回合時間線</h4>
              <ol>
                {roundTimeline.map((item) => (
                  <li key={item.key}>{item.text}</li>
                ))}
              </ol>
            </section>
          )}
          {p.brief && (
            <section>
              <h4>需求摘要</h4>
              <p>{p.brief}</p>
              {p.model_changed?.brief !== undefined && (
                <p className="note">模型改過這一段（需求摘要），原本是：{p.model_changed.brief}</p>
              )}
              <h5>驗收條件</h5>
              {p.acceptance_criteria.length > 0 ? (
                <ol>
                  {p.acceptance_criteria.map((c, i) => (
                    <li key={i}>{c}</li>
                  ))}
                </ol>
              ) : (
                <p>模型尚未提出驗收條件</p>
              )}
              {p.model_changed?.acceptance_criteria !== undefined && (
                <>
                  <p className="note">模型改過這一段（驗收條件），原本是：</p>
                  <ol className="note">
                    {p.model_changed.acceptance_criteria.map((c, i) => (
                      <li key={i}>{c}</li>
                    ))}
                  </ol>
                </>
              )}
              {p.sample_input && (
                <>
                  <h5>試跑用的範例輸入</h5>
                  <pre className="skill-md">
                    <Reveal text={p.sample_input} />
                  </pre>
                </>
              )}
              {p.model_changed?.sample_input !== undefined && (
                <>
                  <p className="note">模型改過這一段（範例輸入），原本是：</p>
                  <pre className="skill-md">
                    <Reveal text={p.model_changed.sample_input} />
                  </pre>
                </>
              )}
              <p>{p.brief_confirmed ? "需求摘要與驗收條件皆已確認" : "尚未確認"}</p>
              {p.pending_action === "confirm_brief" && (
                <button disabled={locked} onClick={() => void perform("confirm_brief")}>
                  {p.model_changed ? "我看過差異，確認新的需求摘要" : "確認需求摘要與驗收條件"}
                </button>
              )}
            </section>
          )}
          {/* 「收到了沒有」現在由對話自己回答（圖坐在它所屬的那一輪裡），所以這裡
              只剩對話說不出口的那一件：圖收到了、但那一步中斷、理解沒生出來。 */}
          {session?.state === "needs_reupload" && (
            <p>這一步中斷了，Agent 沒能讀出那張圖；請在下面重新上傳同一張。</p>
          )}
          {p.diagram_understanding && (
            <section>
              <h4>流程圖理解</h4>
              <DiagramUnderstandingView raw={p.diagram_understanding} />
              <p>{p.diagram_confirmed ? "已確認" : "尚未確認"}</p>
              {p.pending_action === "confirm_diagram" && (
                <button
                  disabled={locked || !parseDiagramUnderstanding(p.diagram_understanding)}
                  onClick={() => void perform("confirm_diagram")}
                >
                  確認流程圖理解
                </button>
              )}
            </section>
          )}
          {p.pending_action === "confirm_fetch" && p.pending_fetch_url && (
            <section>
              <h4>連網讀取確認</h4>
              <p>
                模型想連到 <code>{p.pending_fetch_url}</code>{" "}
                讀取內容來補資料。你的網路環境可能擋住這個網站；被擋住時會直接回報，不會重試。
              </p>
              <button disabled={locked} onClick={() => void perform("confirm_fetch")}>
                同意連網
              </button>
              <button disabled={locked} onClick={() => void perform("decline_fetch")}>
                不連網
              </button>
            </section>
          )}
          {!!p.fetches?.length && (
            <section>
              <h4>已讀取的網頁</h4>
              <ul>
                {p.fetches.map((f, i) => (
                  <li key={i}>
                    {f.url}：{FETCH_STATUS_LABEL[f.status] ?? f.status}
                    {f.bytes !== undefined && `（${f.bytes} 位元組）`}
                  </li>
                ))}
              </ul>
            </section>
          )}
          {(p.references.length > 0 || p.pending_action === "confirm_references") && (
            <section>
              <h4>參考 Skill</h4>
              {p.pending_action === "confirm_references" && p.catalog_checked && (
                <p>目錄裡已有相近的 Skill；你可以直接採用其中一個、以它們為參考，或從頭寫。</p>
              )}
              <div className="table-scroll">
                <table>
                  <thead>
                    <tr>
                      <th>Skill</th>
                      <th>摘要</th>
                      <th>相容</th>
                      <th>工具</th>
                      <th>版本</th>
                      <th>層級</th>
                      <th>掃描</th>
                      <th>狀態</th>
                    </tr>
                  </thead>
                  <tbody>
                    {p.references.map((r) => (
                      <tr key={r.skill_id}>
                        <th scope="row">{r.name}</th>
                        <td>{declaredReferenceField(r.description)}</td>
                        <td>{declaredReferenceField(r.compatibility)}</td>
                        <td>{declaredReferenceField(r.allowed_tools)}</td>
                        <td>
                          <details>
                            <summary>固定版本</summary>
                            {r.version_id}
                          </details>
                        </td>
                        <td>{referenceTierLabel(r.tier)}</td>
                        <td>{referenceScanLabel(r.scan_status, r.warnings)}</td>
                        <td>
                          {!r.available ? "目前不可用" : r.confirmed ? "已確認" : "尚未確認"}
                          {p.pending_action === "confirm_references" && (
                            <>
                              <button
                                disabled={locked || !r.available}
                                onClick={() =>
                                  void perform("adopt_reference", {
                                    reference_skill_ids: [r.skill_id],
                                  })
                                }
                              >
                                直接採用
                              </button>
                              {r.scan_status !== "scanned" && (
                                <span className="note">沒有掃描紀錄，不建議直接採用</span>
                              )}
                            </>
                          )}
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
              {p.pending_action === "confirm_references" && (
                <>
                  <button
                    disabled={locked || p.references.some((r) => !r.available)}
                    onClick={() => void perform("confirm_references")}
                  >
                    以這些為參考
                  </button>
                  <button disabled={locked} onClick={() => void perform("decline_references")}>
                    都不是，從頭寫
                  </button>
                </>
              )}
            </section>
          )}
          {p.pending_action === "confirm_duplicate" && p.duplicates && p.duplicates.length > 0 && (
            <section>
              <h4>目錄已有相近的 Skill</h4>
              <p>
                保存前 Go
                查了一次目錄：下面這些和你的草稿很接近。你可以直接採用其中一個，或仍然建立自己的版本。
              </p>
              <div className="table-scroll">
                <table>
                  <thead>
                    <tr>
                      <th>Skill</th>
                      <th>摘要</th>
                      <th>相容</th>
                      <th>工具</th>
                      <th>版本</th>
                      <th>層級</th>
                      <th>掃描</th>
                      <th></th>
                    </tr>
                  </thead>
                  <tbody>
                    {p.duplicates.map((r) => (
                      <tr key={r.skill_id}>
                        <th scope="row">{r.name}</th>
                        <td>{declaredReferenceField(r.description)}</td>
                        <td>{declaredReferenceField(r.compatibility)}</td>
                        <td>{declaredReferenceField(r.allowed_tools)}</td>
                        <td>
                          <details>
                            <summary>固定版本</summary>
                            {r.version_id}
                          </details>
                        </td>
                        <td>{referenceTierLabel(r.tier)}</td>
                        <td>{referenceScanLabel(r.scan_status, r.warnings)}</td>
                        <td>
                          <button
                            disabled={locked || !r.available}
                            onClick={() =>
                              void perform("adopt_reference", { reference_skill_ids: [r.skill_id] })
                            }
                          >
                            直接採用
                          </button>
                          {r.scan_status !== "scanned" && (
                            <span className="note">沒有掃描紀錄，不建議直接採用</span>
                          )}
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
              <button
                disabled={locked || !p.draft?.content_hash}
                onClick={() =>
                  void perform("confirm_duplicate", { content_hash: p.draft!.content_hash })
                }
              >
                仍然建立
              </button>
            </section>
          )}
          {p.draft && (
            <section>
              <h4>Skill 草稿：{p.draft.skill.name}</h4>
              <p>{p.draft.skill.description}</p>
              <p>
                允許工具：{p.draft.skill.allowed_tools || "未宣告"}。相容條件：
                {p.draft.skill.compatibility || "未宣告"}。
              </p>
              {p.previous_draft && (
                <details>
                  <summary>比較上一份草稿（revision {p.previous_draft.revision}）</summary>
                  <pre className="skill-md">
                    <Reveal text={p.previous_draft.skill.body} />
                  </pre>
                  {p.previous_draft.skill.files.map((f) => (
                    <pre key={f.path}>{f.path + "\n" + f.content}</pre>
                  ))}
                </details>
              )}
              <pre className="skill-md">
                <Reveal text={p.draft.skill.body} />
              </pre>
              {p.draft.skill.files.map((f) => (
                <details key={f.path}>
                  <summary>{f.path}</summary>
                  <pre className="skill-md">
                    <Reveal text={f.content} />
                  </pre>
                </details>
              ))}
              <DraftFindings raw={p.draft.validation} />
              <p>
                {p.draft.blocked
                  ? "靜態檢查阻擋保存，請補充需求後修訂。"
                  : "已完成靜態檢查；這不代表試跑成功。"}
              </p>
              {!p.candidate && p.pending_action !== "confirm_duplicate" && (
                <button
                  disabled={locked || p.draft.blocked || !p.draft.content_hash}
                  onClick={() =>
                    void perform("materialize", { content_hash: p.draft!.content_hash })
                  }
                >
                  建立私人候選版本
                </button>
              )}
              {p.candidate && (
                <>
                  {p.adopted && (
                    <p>已直接採用現有 Skill；這個候選版本是它的複本，沒有生成任何內容。</p>
                  )}
                  <p>
                    <Link
                      to="/lab/run"
                      search={{
                        skill: p.candidate.skill_id,
                        version: p.candidate.version_id,
                        test_case: p.candidate.test_case_id,
                      }}
                    >
                      檢查權限與費用後試跑此版本
                    </Link>
                  </p>
                  {p.candidate.test_case_id && <p>已依確認的驗收條件建立 Test Case</p>}
                  {p.candidate.run_id ? (
                    <>
                      <Link to="/runs/$runId" params={{ runId: p.candidate.run_id }}>
                        查看這次 Run 結果
                      </Link>
                      {run && (
                        <p>
                          試跑結果：{run.execution_status}；評估：
                          {run.evaluation?.overall ?? "無評估"}
                        </p>
                      )}
                    </>
                  ) : (
                    <p>目前尚未連結試跑結果。</p>
                  )}
                  {!terminal &&
                    (latest ? (
                      latest.run_id === p.candidate.run_id ? (
                        <p>最新試跑結果已帶回會話；模型的建議在對話裡。</p>
                      ) : (
                        <>
                          <p>
                            最新試跑：{runStatusLabel(latest.status)}；評估：
                            {latest.evaluation.label}
                          </p>
                          <button
                            disabled={locked}
                            onClick={() => void perform("attach_run", { run_id: latest.run_id })}
                          >
                            把最新試跑結果帶回來改善
                          </button>
                        </>
                      )
                    ) : (
                      <p>試跑完成後，這裡會出現「把最新試跑結果帶回來改善」。</p>
                    ))}
                </>
              )}
              {!terminal && (
                <>
                  <p>
                    保存將採用目前顯示的草稿與版本。{!p.candidate?.run_id && "這份草稿尚未試跑。"}
                    {runNotPassing && "試跑未通過或未評估；保存前請確認。"}
                  </p>
                  {/* 這一頁唯一的填色主要動作（設計 §4.6.3，2026-09-09 入表）。
                      判準是「完成這一頁的工作的那一個」，而這一頁的工作是把一個
                      Skill 做出來並收進工作區——保存就是那一下。在這之前它與同畫面
                      的十顆按鈕同框，於是「送出」「取消」「停止這一步」和「保存」
                      看起來一樣重。填色只有這一顆，`rendered.spec.ts` 數的就是它。 */}
                  <button
                    className="action"
                    disabled={locked || p.draft.blocked || !p.draft.content_hash}
                    onClick={() =>
                      void perform("finalize", { content_hash: p.draft!.content_hash })
                    }
                  >
                    確認保存到私人工作區
                  </button>
                </>
              )}
              {session?.state === "saved" && p.candidate && (
                <Link to="/skills/$skillId" params={{ skillId: p.candidate.skill_id }}>
                  開啟已保存的 Skill
                </Link>
              )}
            </section>
          )}
          {!terminal && (
            <button disabled={busy} onClick={() => void perform("cancel")}>
              取消這次創作
            </button>
          )}
        </>
      )}
      {/* ── 2026-09-08：輸入區在對話下面 ────────────────────────────────────
          在這之前它在對話紀錄**上面**：你得先打字，捲下去才看得到剛才講了什麼。
          對話介面的順序是「先看說了什麼，再說下一句」。 */}
      {!terminal && (
        /*
         * ── 2026-09-08：三個入口收成一個輸入區 ──────────────────────────────
         * 在這之前這裡是一組 radio（自然語言／流程圖／目錄參考）＋ 三個互斥的欄位：
         * 要附流程圖得先切換模式，切過去文字框就不見了。負責人的話：「不應該是拆開
         * 來多個 UI 項目」——對，這三個不是三種模式，是同一件事的三種素材。
         *
         * 現在文字框永遠在，底下一列是附加動作；**素材種類由你放了什麼推出來**，
         * 不再由你先選。`mode` 這個 state 因此消失。
         *
         * **一次只送一種，這一條不是版面選擇**：平台的 action 是 `message`／
         * `diagram`／`select_references` 三個不同的 kind，一次呼叫只帶一個，而送完
         * 一輪會話就進 working、下一個 action 要等新的 revision。所以同時放了兩種
         * 素材時這裡擋下來並說清楚順序，而不是假裝送得出去然後失敗。
         */
        /*
         * ── 2026-09-09：鍵盤、剪貼簿、拖放 ─────────────────────────────────
         * 三件都是聊天介面的通則，這裡在此之前一件都沒有：送出只能用滑鼠點按鈕，
         * 圖只能經由檔案對話框。**貼上尤其重要**——流程圖多半是一張截圖。
         */
        <div
          className="composer"
          data-dragging={dragging || undefined}
          onDragOver={(e) => {
            if (locked) return;
            e.preventDefault();
            setDragging(true);
          }}
          onDragLeave={() => setDragging(false)}
          onDrop={(e) => {
            e.preventDefault();
            setDragging(false);
            if (!locked) chooseFile(e.dataTransfer.files[0]);
          }}
        >
          <label>
            <span className="creation-who">想完成的任務</span>
            {/* 沒有 `maxLength`，而且是刻意的：瀏覽器數的是 UTF-16 code unit，
                伺服器數的是 rune，於是同一段字兩邊的界線不同——而 `maxLength`
                的執行方式是**無聲截斷**，把人寫的字剪掉卻不說。改成報數（下面
                那一行）＋送出前一句明話。`GenerateSkill.tsx` 的任務描述早就是
                這個配方，只有這裡不是。 */}
            <textarea
              aria-label="想完成的任務"
              aria-describedby="composer-count"
              value={message}
              onChange={(e) => setMessage(e.target.value)}
              onKeyDown={(e) => {
                // `isComposing`：注音／倉頡選字時按 Enter 是「確定這個字」，不是
                // 「送出」。少了這一條，中文使用者每打一個字就送出一次。
                if (e.key !== "Enter" || e.shiftKey || e.nativeEvent.isComposing) return;
                e.preventDefault();
                if (!locked && !creditsBlocked) void submit();
              }}
              onPaste={(e) => {
                const picked = [...e.clipboardData.files].find((f) => f.type.startsWith("image/"));
                if (!picked || locked) return;
                e.preventDefault();
                chooseFile(picked);
              }}
              disabled={busy}
              placeholder="要完成什麼、輸入是什麼、預期產出是什麼。"
            />
          </label>
          <p className="note field-count" id="composer-count">
            {[...message].length.toLocaleString("zh-TW")} /{" "}
            {MAX_MESSAGE_RUNES.toLocaleString("zh-TW")} 字
            {[...message].length > MAX_MESSAGE_RUNES && "——超過了，送出會被擋下"}
          </p>
          {/* 兩個上限說在控制項**之前**，而不是等 4xx 才說（設計 §2.2 第二向）。
              位置從送出鍵之後搬上來：一句「你只能附這麼大的圖」出現在你按下送出
              之後，就不是在講上限，是在解釋失敗。`aria-describedby` 把它綁在兩個
              控制項上，`GenerateSkill.tsx` 對同一種素材本來就是這個配方。 */}
          <p className="note" id="composer-limits">
            Enter 送出，Shift＋Enter 換行；圖可以直接貼上或拖進來。 流程圖限 PNG、JPEG 或 WebP，最多
            4,000,000 位元組（約 3.8 MB）；參考 Skill 最多三個。
            文字說明可以跟著流程圖或參考一起送。
          </p>
          <div className="composer-tools">
            {/* 沒有 `aria-label`：`<label>` 包著這個輸入，可及名稱就是看得見的那五個
                字。原本掛的 `aria-label="流程圖"` 會蓋掉它，於是語音操作念畫面上的
                「附一張流程圖」點不到這個控制項（WCAG 2.5.3）。 */}
            <label className="composer-attach">
              附一張流程圖
              <input
                ref={fileInput}
                type="file"
                accept="image/png,image/jpeg,image/webp"
                aria-describedby="composer-limits"
                disabled={locked}
                onChange={(e) => chooseFile(e.target.files?.[0])}
              />
            </label>
            <button
              type="button"
              aria-expanded={picking}
              aria-controls="composer-references"
              aria-describedby="composer-limits"
              disabled={locked}
              onClick={() => setPicking((v) => !v)}
            >
              參考目錄裡的 Skill{refs.length > 0 && `（${refs.length}）`}
            </button>
            <button
              type="button"
              className="composer-send"
              disabled={locked || creditsBlocked}
              aria-describedby={creditsBlocked ? "creation-credits-why-disabled" : undefined}
              onClick={() => void submit()}
            >
              {busy ? "送出中…" : session ? "送出" : "開始互動創作"}
            </button>
          </div>
          {/* 每一顆 chip 說的是按下去會發生什麼事（「移除」），不是它代表什麼東西。
              原本是「流程圖：flow.png ✕」——一個名詞加一個符號，螢幕閱讀器念出來
              是一份檔名，不是一個動作。 */}
          {(file || refs.length > 0) && (
            <ul className="chip-row">
              {file && (
                <li>
                  <button type="button" disabled={locked} onClick={clearFile}>
                    {/* 縮圖是唯一能回答「我選到的是不是我要的那張」的東西；
                        `alt=""` 因為右邊那句話已經說出它是什麼了。 */}
                    {preview && <img className="chip-thumb" src={preview} alt="" />}
                    移除流程圖：{file.name}
                  </button>
                </li>
              )}
              {refs.map((r) => (
                <li key={r.id}>
                  <button
                    type="button"
                    disabled={locked}
                    onClick={() => setRefs((old) => old.filter((x) => x.id !== r.id))}
                  >
                    移除參考：{r.name}
                  </button>
                </li>
              ))}
            </ul>
          )}
          {picking && (
            <div id="composer-references">
              <ReferencePicker
                disabled={locked}
                references={refs}
                onToggle={(skillID, name) =>
                  setRefs((old) =>
                    old.some((r) => r.id === skillID)
                      ? old.filter((r) => r.id !== skillID)
                      : old.length < 3
                        ? [...old, { id: skillID, name }]
                        : old,
                  )
                }
              />
            </div>
          )}
          {working && <p className="note">正在處理目前素材；完成後即可補充或確認。</p>}
        </div>
      )}
    </div>
  );
}
