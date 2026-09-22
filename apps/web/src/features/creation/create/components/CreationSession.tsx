import { Fragment, useEffect, useMemo, useRef, useState } from "react";
import { Link } from "@tanstack/react-router";
import { ApiError } from "../../../../core/api/client";
import {
  actOnCreationSession,
  createCreationSession,
  useCreationLimits,
  useCreationSessionCache,
  useCreationSessions,
  useLiveCreationSession,
  type CreationAction,
  type CreationSession as Session,
  type CreationState,
} from "../../creation.service";
import { useCredits } from "../../../../core/session/credits.service";
import { useRuns } from "../../../runs";
import { TERMINAL_RUN_STATUSES } from "../../../runs";
import { ReadFailure } from "../../../../shared/ui/LoginRequired";
import { ReferencePicker } from "../../generate/GenerateSkill";
import { ModelMarkdown } from "./ModelMarkdown";
import { Reveal } from "../../../../shared/ui/Reveal";
import { Timestamp } from "../../../../shared/ui/Timestamp";
import { runStatusLabel } from "../../../runs";
import {
  diagramProblem,
  readImage,
  findRunObservation,
  parseDiagramUnderstanding,
  stepDescription,
  FETCH_STATUS_LABEL,
  buildRoundTimeline,
  budgetChoices,
  nextStepBudget,
  newestMessageIn,
  isBelow,
} from "../create.model";
import { DraftFindings } from "./DraftFindings";
import { DiagramUnderstandingView } from "./DiagramUnderstandingView";
import { Attachments } from "./Attachments";
import { ToolObservation } from "./ToolObservation";
import { ReferenceList } from "./ReferenceList";
import "./CreationSession.css";

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

const points = (v: number) => v + " 點";

const newCommandID = () => {
  if (typeof crypto.randomUUID === "function") return crypto.randomUUID();
  const bytes = crypto.getRandomValues(new Uint8Array(16));
  bytes[6] = (bytes[6] & 0x0f) | 0x40;
  bytes[8] = (bytes[8] & 0x3f) | 0x80;
  return [...bytes].map((byte) => byte.toString(16).padStart(2, "0")).join("").replace(
    /(.{8})(.{4})(.{4})(.{4})(.{12})/,
    "$1-$2-$3-$4-$5",
  );
};

function NextStep({
  costCredits,
  remainingCredits,
  roomForAnother,
}: {
  costCredits: number;
  remainingCredits: number;
  roomForAnother: boolean;
}) {
  if (roomForAnother) {
    return (
      <>
        {" "}
        · 下一步最多 {points(costCredits)}，預算還有 {points(remainingCredits)}
      </>
    );
  }
  return (
    <>
      {" "}
      ·{" "}
      <strong>
        預算只剩 {points(remainingCredits)}，不夠再走一步的 {points(costCredits)}
      </strong>
      ，展開可以提高預算
    </>
  );
}

const STARTERS = [
  {
    title: "會議記錄 → 待辦清單",
    desc: "從逐字稿抓出待辦、負責人和期限",
    prompt: "把會議逐字稿整理成待辦清單，每一項要有負責人和期限；沒講到期限就標「未定」。",
  },
  {
    title: "客服來信分類",
    desc: "依問題類型分類，並草擬第一版回覆",
    prompt: "把客服來信依問題類型分類，並為每一封草擬第一版回覆。",
  },
  {
    title: "發票資料擷取",
    desc: "抓出金額、日期與統一編號",
    prompt: "從發票內容擷取金額、開立日期與統一編號，輸出成一張表格。",
  },
  {
    title: "PR → 版本說明",
    desc: "把合併的 PR 整理成給使用者看的更新說明",
    prompt: "把這週合併的 PR 描述整理成給使用者看的版本更新說明，依功能分組。",
  },
];

export function CreationSession() {
  const cache = useCreationSessionCache();
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
  const [lastAttempt, setLastAttempt] = useState<"submit" | [CreationAction["kind"], Extra]>();
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
  const sessions = useCreationSessions();
  const limits = useCreationLimits();
  const current = useLiveCreationSession(id);
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
    cache.remember(value);
  };
  const send = async (value: Session, kind: CreationAction["kind"], extra: Extra = {}) => {
    const key = JSON.stringify([value.id, kind, extra]);
    if (pending.current?.key !== key)
      pending.current = {
        key,
        body: {
          command_id: newCommandID(),
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
        await cache.reload(value.id);
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
      setLastAttempt([kind, extra]);
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
  const stream = useRef<HTMLDivElement>(null);
  const [latestHidden, setLatestHidden] = useState(false),
    [unseen, setUnseen] = useState(0);
  useEffect(() => {
    const el = stream.current;
    if (!el) return;
    const onScroll = () => {
      const hidden = isBelow(newestMessageIn(el), el);
      setLatestHidden(hidden);
      if (!hidden) setUnseen(0);
    };
    el.addEventListener("scroll", onScroll, { passive: true });
    return () => el.removeEventListener("scroll", onScroll);
  }, []);
  const showLatest = () => {
    if (stream.current) newestMessageIn(stream.current)?.scrollIntoView?.({ block: "start" });
    setLatestHidden(false);
    setUnseen(0);
  };
  const messageCount = p?.messages.length ?? 0;
  const newestRole = p?.messages[messageCount - 1]?.role;
  const seen = useRef({ id: "", count: 0 });
  useEffect(() => {
    const before = seen.current;
    seen.current = { id, count: messageCount };
    const reopened = before.id !== id || before.count === 0;
    const added = messageCount - (reopened ? 0 : before.count);
    if (added <= 0) return;
    if (!reopened && latestHidden && newestRole !== "user") setUnseen((n) => n + added);
    else showLatest();
  }, [id, messageCount, newestRole, latestHidden]);
  useEffect(() => {
    if (working)
      stream.current
        ?.querySelector(".creation-log > li[data-pending]")
        ?.scrollIntoView?.({ block: "nearest" });
  }, [working]);
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
            body: { id: newCommandID(), message: initial, budget_credits: amount },
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
      setLastAttempt("submit");
    } finally {
      setBusy(false);
    }
  };
  const retry = () => {
    if (lastAttempt === "submit") void submit();
    else if (lastAttempt) void perform(...lastAttempt);
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
  const failureBox = failure && (
    <div className="notice notice-danger toast">
      {failure}
      {error instanceof TypeError && lastAttempt && (
        <button type="button" disabled={busy} onClick={retry}>
          重試
        </button>
      )}
      <button
        type="button"
        className="toast-close"
        aria-label="關閉"
        onClick={() => setError(undefined)}
      >
        ×
      </button>
    </div>
  );
  const historyMenu = useRef<HTMLDetailsElement>(null);
  const pickSession = (next: string) => {
    setID(next);
    setMessage("");
    clearFile();
    setRefs([]);
    pending.current = undefined;
    setError(undefined);
    if (historyMenu.current) historyMenu.current.open = false;
  };
  const hasContent = message.trim() !== "" || !!file || refs.length > 0;
  const avatar = (
    <span className="agent-avatar" aria-hidden="true">
      ✦
    </span>
  );
  return (
    <div className="creation-shell">
      <header className="creation-bar">
        <nav aria-label="離開這一頁">
          <Link to="/workspace/skills" className="bar-back" aria-label="回到我的 Skill">
            ←
          </Link>
        </nav>
        {avatar}
        <div className="bar-title">
          <h3>和 Agent 一起創作 Skill</h3>
          <span className="creation-state">
            {session ? (
              <>
                <span role="status">{labels[session.state]}</span>
                {p && limits.data && ` · ${p.steps}／${limits.data.max_steps} 步`}
              </>
            ) : (
              "說出任務，一步步做成你的 Skill"
            )}
          </span>
        </div>
        {session && p && (
          <details className="creation-details">
            <summary>
              費用 {p.spent_credits === undefined || p.usage_unknown ? "未知" : p.spent_credits} /{" "}
              {points(p.budget_credits)}
              {limits.data && !terminal && (
                <NextStep {...nextStepBudget(p, limits.data.min_budget_credits)} />
              )}
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
        {sessions.data && sessions.data.length > 0 && (
          <details className="creation-history" ref={historyMenu}>
            <summary>對話紀錄</summary>
            <ul>
              <li>
                <button
                  type="button"
                  disabled={busy}
                  aria-current={!id || undefined}
                  onClick={() => pickSession("")}
                >
                  ＋ 開始新的創作
                </button>
              </li>
              {sessions.data.slice(0, 50).map((s) => (
                <li key={s.id}>
                  <button
                    type="button"
                    data-session={s.id}
                    disabled={busy}
                    aria-current={s.id === id || undefined}
                    onClick={() => pickSession(s.id)}
                  >
                    <span>{s.snapshot.brief.slice(0, 40) || "尚未確認需求"}</span>
                    <span className="note">{labels[s.state]}</span>
                  </button>
                </li>
              ))}
            </ul>
          </details>
        )}
      </header>
      <div className="creation-stream" ref={stream}>
        <div className="creation-feed">
          <ReadFailure error={sessions.error ?? current.error} what="創作紀錄" />
          {!p && (
            <>
              <p className="system-line">Agent 會先和你確認需求與驗收條件，才開始寫草稿</p>
              <ol className="creation-log">
                <li data-role="assistant">
                  {avatar}
                  <span className="creation-who">Agent</span>
                  <span className="creation-text">
                    想做一個什麼樣的 Skill？說說它要完成什麼，也可以附上流程圖。
                  </span>
                </li>
              </ol>
              <ul className="starter-cards" aria-label="可以這樣開始">
                {STARTERS.map((s) => (
                  <li key={s.title}>
                    <button
                      type="button"
                      disabled={busy}
                      onClick={() => {
                        setMessage(s.prompt);
                        textarea.current?.focus();
                      }}
                    >
                      <strong>{s.title}</strong>
                      <span>{s.desc}</span>
                    </button>
                  </li>
                ))}
              </ul>
            </>
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
                        <li data-role={m.role} data-index={i}>
                          {m.role === "assistant" && avatar}
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
                      {avatar}
                      <span className="creation-who">Agent</span>
                      <span className="typing" aria-hidden="true">
                        <span />
                        <span />
                        <span />
                      </span>
                      <span className="creation-text">{stepDescription(p)}</span>
                      <p className="note">
                        這一步會自己結束。可以關掉這一頁，回來時從「對話紀錄」繼續；上次更新{" "}
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
              </div>
              {roundTimeline.length > 0 && (
                <section>
                  <h4>回合時間線</h4>
                  <ol className="round-timeline">
                    {roundTimeline.map((item) => (
                      <li key={item.key}>{item.text}</li>
                    ))}
                  </ol>
                </section>
              )}
              {p.brief && (
                <section>
                  <header className="card-header">
                    <h4>需求摘要</h4>
                    <span className="card-tag" data-tone={p.brief_confirmed ? "done" : undefined}>
                      {p.brief_confirmed ? "已確認" : "尚未確認"}
                    </span>
                  </header>
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
                  {p.pending_action === "confirm_brief" && (
                    <div className="card-actions">
                      <button
                        className="card-primary"
                        disabled={locked}
                        onClick={() => void perform("confirm_brief")}
                      >
                        {p.model_changed
                          ? "我看過差異，確認新的需求摘要"
                          : "確認需求摘要與驗收條件"}
                      </button>
                    </div>
                  )}
                </section>
              )}
              {session?.state === "needs_reupload" && (
                <p className="system-line">
                  這一步中斷了，Agent 沒能讀出那張圖；請在下面重新上傳同一張。
                </p>
              )}
              {p.diagram_understanding && (
                <section>
                  <header className="card-header">
                    <h4>流程圖理解</h4>
                    <span className="card-tag" data-tone={p.diagram_confirmed ? "done" : undefined}>
                      {p.diagram_confirmed ? "已確認" : "尚未確認"}
                    </span>
                  </header>
                  <DiagramUnderstandingView raw={p.diagram_understanding} />
                  {p.pending_action === "confirm_diagram" && (
                    <div className="card-actions">
                      <button
                        className="card-primary"
                        disabled={locked || !parseDiagramUnderstanding(p.diagram_understanding)}
                        onClick={() => void perform("confirm_diagram")}
                      >
                        確認流程圖理解
                      </button>
                    </div>
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
                  <div className="card-actions">
                    <button disabled={locked} onClick={() => void perform("decline_fetch")}>
                      不連網
                    </button>
                    <button
                      className="card-primary"
                      disabled={locked}
                      onClick={() => void perform("confirm_fetch")}
                    >
                      同意連網
                    </button>
                  </div>
                </section>
              )}
              {!!p.fetches?.length && (
                <section>
                  <h4>已讀取的網頁</h4>
                  <ul className="ref-facts">
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
                  <ReferenceList
                    items={p.references}
                    adoptable={p.pending_action === "confirm_references"}
                    showStatus
                    locked={locked}
                    onAdopt={(skillID) =>
                      void perform("adopt_reference", { reference_skill_ids: [skillID] })
                    }
                  />
                  {p.pending_action === "confirm_references" && (
                    <div className="card-actions">
                      <button disabled={locked} onClick={() => void perform("decline_references")}>
                        都不是，從頭寫
                      </button>
                      <button
                        className="card-primary"
                        disabled={locked || p.references.some((r) => !r.available)}
                        onClick={() => void perform("confirm_references")}
                      >
                        以這些為參考
                      </button>
                    </div>
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
                    <ReferenceList
                      items={p.duplicates}
                      adoptable
                      showStatus={false}
                      locked={locked}
                      onAdopt={(skillID) =>
                        void perform("adopt_reference", { reference_skill_ids: [skillID] })
                      }
                    />
                    <div className="card-actions">
                      <button
                        className="card-primary"
                        disabled={locked || !p.draft?.content_hash}
                        onClick={() =>
                          void perform("confirm_duplicate", { content_hash: p.draft!.content_hash })
                        }
                      >
                        仍然建立
                      </button>
                    </div>
                  </section>
                )}
              {p.draft && (
                <section>
                  <header className="card-header">
                    <h4>Skill 草稿：{p.draft.skill.name}</h4>
                    <span className="card-tag" data-tone={p.draft.blocked ? "danger" : "done"}>
                      {p.draft.blocked ? "靜態檢查阻擋保存" : "已完成靜態檢查"}
                    </span>
                  </header>
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
                  <p className="note">
                    {p.draft.blocked ? "請補充需求後修訂。" : "靜態檢查通過不代表試跑成功。"}
                  </p>
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
                      <div className="card-actions">
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
                        <button
                          className="action"
                          disabled={locked || p.draft.blocked || !p.draft.content_hash}
                          onClick={() =>
                            void perform("finalize", { content_hash: p.draft!.content_hash })
                          }
                        >
                          確認保存到私人工作區
                        </button>
                      </div>
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
          {latestHidden && !failureBox && (
            <button type="button" className="to-latest" onClick={showLatest}>
              ↓ {unseen > 0 ? `${unseen} 則新訊息` : "回到最新"}
            </button>
          )}
          {!session && (creditsBlocked || !!limits.error) && (
            <p className="notice notice-danger" id="composer-why">
              {creditsBlocked
                ? credits.data?.block_reason
                : "讀不到這次可用的預算範圍，暫時不能開始。"}
            </p>
          )}
          {!session && choices.length > 0 && (
            <fieldset className="budget-picker">
              <legend>這次預算上限</legend>
              <div className="quick-replies">
                {choices.map((v) => (
                  <label key={v}>
                    <input
                      type="radio"
                      name="creation-budget"
                      value={v}
                      checked={budget === String(v)}
                      disabled={busy}
                      onChange={(e) => setBudget(e.target.value)}
                    />
                    {points(v)}
                  </label>
                ))}
              </div>
              {credits.data && !creditsBlocked && (
                <span className="creation-fact">
                  餘額 {credits.data.balance_credits} 點 · 這場約{" "}
                  {credits.data.estimated_session.low_credits}–
                  {credits.data.estimated_session.high_credits} 點
                  {credits.data.estimated_session.estimated && "（估計）"}
                </span>
              )}
            </fieldset>
          )}
          <div
            className="composer"
            data-dragging={dragging || undefined}
            data-empty={!hasContent || undefined}
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
                    ? "先在上方選這次的預算上限"
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
                ＋ 參考 Skill{refs.length > 0 && `（${refs.length}）`}
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
            {failureBox}
          </div>
          <span id="composer-limits">
            流程圖可以貼上或拖進來：PNG、JPEG、WebP，最多 4,000,000 位元組（約 3.8 MB）；參考 Skill
            最多三個。
          </span>
        </div>
      )}
      {terminal && failureBox && <div className="composer-dock">{failureBox}</div>}
    </div>
  );
}
