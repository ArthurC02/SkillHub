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
const MAX_MESSAGE_RUNES = 4000;
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
    } catch {}
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
      } catch {}
    } else if (m.role === "tool" && m.content.startsWith('{"fetch"')) {
      try {
        const parsed = JSON.parse(m.content) as { fetch: { url: string; status: string } };
        items.push({
          key: `t-${i}`,
          text: `讀取網頁：${parsed.fetch.url}（${FETCH_STATUS_LABEL[parsed.fetch.status] ?? parsed.fetch.status}）`,
        });
      } catch {}
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
function budgetChoices(min: number, max: number) {
  return [...new Set([min, 200, 500, 1000, 2000, 5000, max])]
    .filter((v) => v >= min && v <= max)
    .sort((a, b) => a - b);
}
const points = (v: number) => v + " 點";
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
    [error, setError] = useState<unknown>(),
    [busy, setBusy] = useState(false);
  const fileInput = useRef<HTMLInputElement>(null);
  const textarea = useRef<HTMLTextAreaElement>(null);
  useEffect(() => textarea.current?.focus(), [budget]);
  const clearFile = () => {
    setFile(undefined);
    if (fileInput.current) fileInput.current.value = "";
  };
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
  const preview = useMemo(() => (file ? URL.createObjectURL(file) : undefined), [file]);
  useEffect(
    () => () => {
      if (preview) URL.revokeObjectURL(preview);
    },
    [preview],
  );
  const pending = useRef<{ key: string; body: CreationAction } | undefined>(undefined);
  const startPending = useRef<
    { key: string; body: { id: string; message: string; budget_credits: number } } | undefined
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
    return streamCreationSession(
      id,
      (s) => client.setQueryData(["creation-session", id], s),
      setStreaming,
    );
  }, [id, client]);
  const session = current.data,
    p = session?.snapshot;
  const credits = useCredits();
  const creditsBlocked = !session && !!credits.data && !credits.data.can_start;
  const runs = useRuns(p?.candidate?.test_case_id, Boolean(p?.candidate?.test_case_id));
  const latest = runs.data?.pages[0]?.runs.find((r) => TERMINAL_RUN_STATUSES.has(r.status));
  const run = p?.candidate?.run_id ? findRunObservation(p.messages, p.candidate.run_id) : undefined;
  const roundTimeline = p ? buildRoundTimeline(p.messages) : [];
  const lastSaid = [...(p?.messages ?? [])].reverse().find((m) => m.role === "user")?.content ?? "";
  const runNotPassing =
    !!run && (run.execution_status !== "succeeded" || run.evaluation?.overall !== "met");
  const terminal = !!session && ["saved", "cancelled"].includes(session.state);
  const working = !!session && ["queued", "working"].includes(session.state);
  const locked = busy || working || terminal;
  const choices = limits.data
    ? budgetChoices(limits.data.min_budget_credits, limits.data.max_budget_credits)
    : [];
  const budgetCredits = Number(budget) || undefined;
  const frozen = !session && (budgetCredits === undefined || creditsBlocked);
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
    if (
      !Number.isInteger(amount) ||
      amount <= p.budget_credits ||
      amount > limits.data.max_budget_credits
    ) {
      setError(
        new Error(
          `請填寫高於目前上限 ${p.budget_credits} 點且不超過 ${limits.data.max_budget_credits} 點的點數。`,
        ),
      );
      return;
    }
    await perform("raise_budget", { budget_credits: amount });
  };
  const bottom = useRef<HTMLDivElement>(null);
  const stream = useRef<HTMLDivElement>(null);
  const atBottom = useRef(true);
  useEffect(() => {
    const el = stream.current;
    if (!el) return;
    const onScroll = () => {
      atBottom.current = el.scrollTop + el.clientHeight >= el.scrollHeight - 200;
    };
    el.addEventListener("scroll", onScroll, { passive: true });
    return () => el.removeEventListener("scroll", onScroll);
  }, []);
  const messageCount = p?.messages.length ?? 0;
  useEffect(() => {
    if (messageCount === 0 || !atBottom.current) return;
    // scrollIntoView can land short on the render where the container's own
    // height just changed; setting scrollTop directly is the reliable fallback.
    bottom.current?.scrollIntoView?.({ block: "nearest" });
    const el = stream.current;
    if (el) el.scrollTop = el.scrollHeight;
  }, [messageCount]);
  const submit = async () => {
    setBusy(true);
    setError(undefined);
    try {
      const note = message.trim();
      const mode = file ? "diagram" : refs.length > 0 ? "references" : "message";
      if (!file && refs.length === 0 && !note)
        throw new Error("還沒有要送出的內容：寫一句話，或附上流程圖、挑一個參考 Skill。");
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
        if (budgetCredits === undefined) return;
        const amount = budgetCredits;
        const initial = mode === "message" ? message : "";
        const key = JSON.stringify([initial, amount]);
        if (startPending.current?.key !== key)
          startPending.current = {
            key,
            body: { id: crypto.randomUUID(), message: initial, budget_credits: amount },
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
      if (mode === "diagram") {
        const next = await send(value, "diagram", { diagram, ...(note ? { message: note } : {}) });
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
  const failure = !!error && (
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
  );
  return (
    <div className="creation-shell">
      <div className="creation-bar">
        <nav aria-label="離開這一頁">
          <Link to="/workspace/skills">← 回到我的 Skill</Link>
        </nav>
        <h3>和 Agent 一起創作 Skill</h3>
        {sessions.data && sessions.data.length > 0 && (
          <label className="creation-picker">
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
        {session && (
          <span className="creation-state">
            <span role="status">{labels[session.state]}</span>
            {p && limits.data && ` · ${p.steps}／${limits.data.max_steps} 步`}
          </span>
        )}
        {session && p && (
          <details className="creation-details">
            <summary>
              費用 {p.spent_credits === undefined || p.usage_unknown ? "未知" : p.spent_credits} /{" "}
              {points(p.budget_credits)}
            </summary>
            <div>
              <p className="note">
                仍占用預算 {p.reserved_credits} 點
                {limits.data && (
                  <>
                    {" "}
                    · 工具 {p.tool_calls}／{limits.data.max_tool_calls} 次
                  </>
                )}
              </p>
              {!terminal && (
                <p className="note">
                  可進行到 <Timestamp at={session.deadline} /> · 紀錄保留到{" "}
                  <Timestamp at={session.expires_at} />
                </p>
              )}
              {limits.data && (
                <>
                  <label>
                    提高這次預算上限（點）
                    <input
                      aria-label="提高這次預算上限（點）"
                      inputMode="decimal"
                      disabled={busy}
                      value={raiseBudget}
                      onChange={(e) => setRaiseBudget(e.target.value)}
                    />
                  </label>
                  <button type="button" disabled={busy} onClick={() => void submitRaiseBudget()}>
                    提高預算後繼續
                  </button>
                </>
              )}
              {!terminal && !working && session.state !== "failed" && (
                <button
                  type="button"
                  className="destructive"
                  disabled={busy}
                  onClick={() => void perform("cancel")}
                >
                  取消這次創作
                </button>
              )}
            </div>
          </details>
        )}
        {!session && choices.length > 0 && (
          <label className="creation-picker">
            預算上限
            <select
              aria-label="這次預算上限（點）"
              value={budget}
              disabled={busy}
              onChange={(e) => setBudget(e.target.value)}
            >
              <option value="" disabled>
                請選擇
              </option>
              {choices.map((v) => (
                <option key={v} value={v}>
                  {points(v)}
                </option>
              ))}
            </select>
            {credits.data && !creditsBlocked && (
              <span className="creation-fact">
                餘額 {credits.data.balance_credits} 點 · 這場約{" "}
                {credits.data.estimated_session.low_credits}–
                {credits.data.estimated_session.high_credits} 點
                {credits.data.estimated_session.estimated && "（估計）"}
              </span>
            )}
          </label>
        )}
      </div>
      <div className="creation-stream" ref={stream}>
        <div className="creation-feed">
          <ReadFailure error={sessions.error ?? current.error} what="創作紀錄" />
          {!p && (
            <ol className="creation-log">
              <li data-role="assistant">
                <span className="creation-who">Agent</span>
                <span className="creation-text">
                  想做一個什麼樣的 Skill？說說它要完成什麼，也可以附上流程圖。
                </span>
              </li>
            </ol>
          )}

          {p && (
            <>
              <div role="log" aria-label="與 Agent 的對話">
                <ol className="creation-log">
                  {p.messages.map((m, i) => {
                    const here = (p.attachments ?? []).filter((a) => a.message_index === i);
                    return (
                      <Fragment key={i}>
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
                          {session?.state === "failed" &&
                            m.role === "assistant" &&
                            i === p.messages.length - 1 && (
                              <div className="turn-actions">
                                <button
                                  type="button"
                                  disabled={busy || !lastSaid}
                                  onClick={() => void perform("message", { message: lastSaid })}
                                >
                                  再送一次上一句
                                </button>
                                <button
                                  type="button"
                                  className="destructive"
                                  disabled={busy}
                                  onClick={() => void perform("cancel")}
                                >
                                  取消這次創作
                                </button>
                              </div>
                            )}
                        </li>
                      </Fragment>
                    );
                  })}
                  {(p.attachments ?? []).some((a) => a.message_index >= p.messages.length) && (
                    <li data-role="user">
                      <span className="creation-who">你</span>
                      <Attachments
                        list={(p.attachments ?? []).filter(
                          (a) => a.message_index >= p.messages.length,
                        )}
                        thumbs={thumbs.current}
                      />
                    </li>
                  )}
                  {working && p && (
                    <li data-role="assistant" data-pending="">
                      <span className="creation-who">Agent</span>
                      <span className="creation-text">{stepDescription(p)}</span>
                      <p className="note">
                        這一步會自己結束。可以關掉這一頁，回來時從「恢復創作」繼續；上次更新{" "}
                        <Timestamp at={session.updated_at} relative />
                      </p>
                      <div className="turn-actions">
                        <button
                          type="button"
                          disabled={busy}
                          onClick={() => void perform("stop_step")}
                        >
                          停止這一步
                        </button>
                        <button
                          type="button"
                          className="destructive"
                          disabled={busy}
                          onClick={() => void perform("cancel")}
                        >
                          取消這次創作
                        </button>
                      </div>
                    </li>
                  )}
                </ol>
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
                    <p className="note">
                      模型改過這一段（需求摘要），原本是：{p.model_changed.brief}
                    </p>
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
              {p.pending_action === "confirm_duplicate" &&
                p.duplicates &&
                p.duplicates.length > 0 && (
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
                                onClick={() =>
                                  void perform("attach_run", { run_id: latest.run_id })
                                }
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
                        保存將採用目前顯示的草稿與版本。
                        {!p.candidate?.run_id && "這份草稿尚未試跑。"}
                        {runNotPassing && "試跑未通過或未評估；保存前請確認。"}
                      </p>
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
            </>
          )}
        </div>
      </div>
      {!terminal && (
        <div className="composer-dock">
          {!session && (creditsBlocked || !!limits.error) && (
            <p className="notice notice-danger" id="composer-why">
              {creditsBlocked
                ? credits.data?.block_reason
                : "讀不到這次可用的預算範圍，暫時不能開始。"}
            </p>
          )}
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
              <textarea
                ref={textarea}
                aria-label="想完成的任務"
                aria-describedby="composer-count composer-limits"
                value={message}
                onChange={(e) => setMessage(e.target.value)}
                onKeyDown={(e) => {
                  // isComposing: an IME's Enter confirms the selected character,
                  // it doesn't mean submit.
                  if (e.key !== "Enter" || e.shiftKey || e.nativeEvent.isComposing) return;
                  e.preventDefault();
                  if (!locked && !frozen) void submit();
                }}
                onPaste={(e) => {
                  const picked = [...e.clipboardData.files].find((f) =>
                    f.type.startsWith("image/"),
                  );
                  if (!picked || locked) return;
                  e.preventDefault();
                  chooseFile(picked);
                }}
                disabled={busy || frozen}
                placeholder={
                  frozen && !creditsBlocked && choices.length > 0
                    ? "先在右上角選擇這次的預算上限"
                    : "描述想完成的任務（Enter 送出，Shift＋Enter 換行）"
                }
              />
            </label>
            <div className="composer-tools">
              <label className="composer-attach">
                ＋ 流程圖
                <input
                  ref={fileInput}
                  type="file"
                  accept="image/png,image/jpeg,image/webp"
                  aria-describedby="composer-limits"
                  disabled={locked || frozen}
                  onChange={(e) => chooseFile(e.target.files?.[0])}
                />
              </label>
              <button
                type="button"
                aria-expanded={picking}
                aria-controls="composer-references"
                aria-describedby="composer-limits"
                disabled={locked || frozen}
                onClick={() => setPicking((v) => !v)}
              >
                ＋ 參考目錄 Skill{refs.length > 0 && `（${refs.length}）`}
              </button>
              <span className="note field-count" id="composer-count">
                {[...message].length.toLocaleString("zh-TW")} /{" "}
                {MAX_MESSAGE_RUNES.toLocaleString("zh-TW")} 字
                {[...message].length > MAX_MESSAGE_RUNES && "——超過了，送出會被擋下"}
              </span>
              <button
                type="button"
                className="composer-send"
                disabled={locked || frozen}
                aria-describedby={
                  !session && (creditsBlocked || !!limits.error) ? "composer-why" : undefined
                }
                onClick={() => void submit()}
              >
                {busy ? "送出中…" : session ? "送出" : "開始創作"}
              </button>
            </div>
            {(file || refs.length > 0) && (
              <ul className="chip-row">
                {file && (
                  <li>
                    <button type="button" disabled={locked} onClick={clearFile}>
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
                  onToggle={(skillID, name) => {
                    if (refs.some((r) => r.id === skillID))
                      setRefs(refs.filter((r) => r.id !== skillID));
                    else if (refs.length >= 3)
                      setError(new Error("參考 Skill 最多三個；先移除一個再加。"));
                    else setRefs([...refs, { id: skillID, name }]);
                  }}
                />
              </div>
            )}
          </div>
          <span id="composer-limits">
            流程圖可以貼上或拖進來：PNG、JPEG、WebP，最多 4,000,000 位元組（約 3.8 MB）；參考 Skill
            最多三個。
          </span>
        </div>
      )}
      {failure && (
        <div className="toast">
          {failure}
          <button type="button" onClick={() => setError(undefined)}>
            關閉
          </button>
        </div>
      )}
    </div>
  );
}
