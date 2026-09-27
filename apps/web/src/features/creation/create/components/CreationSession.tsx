import { useEffect, useMemo, useRef, useState } from "react";
import { ApiError } from "../../../../core/api/client";
import {
  useCreationLimits,
  useCreationSessions,
  useLiveCreationSession,
  type CreationAction,
} from "../../creation.service";
import { useCredits } from "../../../../core/session/credits.service";
import { useRuns } from "../../../runs";
import { TERMINAL_RUN_STATUSES } from "../../../runs";
import { ReadFailure } from "../../../../shared/ui/LoginRequired";
import {
  diagramProblem,
  readImage,
  buildRoundTimeline,
  budgetChoices,
  MAX_MESSAGE_RUNES,
} from "../create.model";
import { useCreationCommands, type CommandExtra, type Perform } from "../create.commands";
import { useMessageStream } from "../create.stream";
import { ConversationLog } from "./ConversationLog";
import { SessionHeader } from "./SessionHeader";
import { SessionEmptyState } from "./SessionEmptyState";
import {
  BriefCard,
  DiagramDescriptionCard,
  DiagramInterpretationCard,
  DiagramUnderstandingCard,
  DuplicatesCard,
  FetchConsentCard,
  FetchedPages,
  ReferencesCard,
  RoundTimeline,
} from "./SessionCards";
import { DraftCard } from "./DraftCard";
import { Composer } from "./Composer";
import "./CreationSession.css";

export function CreationSession() {
  const [id, setID] = useState(""),
    [picking, setPicking] = useState(false),
    [dragging, setDragging] = useState(false),
    [message, setMessage] = useState(""),
    [budget, setBudget] = useState(""),
    [file, setFile] = useState<File>(),
    [refs, setRefs] = useState<{ id: string; name: string }[]>([]),
    [raiseBudget, setRaiseBudget] = useState(""),
    [diagramAnswers, setDiagramAnswers] = useState<Record<string, string>>({}),
    [error, setError] = useState<unknown>(),
    [busy, setBusy] = useState(false);
  const [lastAttempt, setLastAttempt] = useState<
    "submit" | [CreationAction["kind"], CommandExtra]
  >();
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
  const [thumbs] = useState(() => new Map<string, string>());
  useEffect(() => () => thumbs.forEach((url) => URL.revokeObjectURL(url)), [thumbs]);
  const preview = useMemo(() => (file ? URL.createObjectURL(file) : undefined), [file]);
  useEffect(
    () => () => {
      if (preview) URL.revokeObjectURL(preview);
    },
    [preview],
  );
  const commands = useCreationCommands(setID);
  const sessions = useCreationSessions();
  const limits = useCreationLimits();
  const current = useLiveCreationSession(id);
  const session = current.data,
    p = session?.snapshot;
  const credits = useCredits();
  const creditsBlocked = !session && !!credits.data && !credits.data.can_start;
  const runs = useRuns(p?.candidate?.test_case_id, Boolean(p?.candidate?.test_case_id));
  const latest = runs.data?.pages[0]?.runs.find((r) => TERMINAL_RUN_STATUSES.has(r.status));
  const roundTimeline = p ? buildRoundTimeline(p.messages) : [];
  const terminal = !!session && ["saved", "cancelled"].includes(session.state);
  const working = !!session && ["queued", "working"].includes(session.state);
  const locked = busy || working || terminal;
  const choices = limits.data
    ? budgetChoices(limits.data.min_budget_credits, limits.data.max_budget_credits)
    : [];
  const budgetCredits = Number(budget) || undefined;
  const frozen = !session && (budgetCredits === undefined || creditsBlocked);
  const perform: Perform = async (kind, extra = {}) => {
    if (!session) return;
    setBusy(true);
    setError(undefined);
    try {
      await commands.send(session, kind, extra);
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
  const messageCount = p?.messages.length ?? 0;
  const { stream, latestHidden, unseen, showLatest } = useMessageStream(
    id,
    messageCount,
    p?.messages[messageCount - 1]?.role,
    working,
  );
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
        value = await commands.start(mode === "message" ? message : "", budgetCredits);
        if (mode === "message") {
          setMessage("");
          return;
        }
      }
      if (mode === "message") {
        await commands.send(value, "message", { message });
        setMessage("");
      }
      if (mode === "diagram") {
        const next = await commands.send(value, "diagram", {
          diagram,
          ...(note ? { message: note } : {}),
        });
        const recorded = next.snapshot.attachments ?? [];
        const mine = recorded[recorded.length - 1];
        if (mine && file) thumbs.set(mine.sha256, URL.createObjectURL(file));
        clearFile();
        setMessage("");
      }
      if (mode === "references") {
        await commands.send(value, "select_references", {
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
    commands.forgetPending();
    setError(undefined);
    if (historyMenu.current) historyMenu.current.open = false;
  };
  const hasContent = message.trim() !== "" || !!file || refs.length > 0;
  return (
    <div className="creation-shell">
      <SessionHeader
        session={session}
        p={p}
        limits={limits.data}
        terminal={terminal}
        working={working}
        busy={busy}
        perform={perform}
        raiseBudget={raiseBudget}
        onRaiseBudget={setRaiseBudget}
        onSubmitRaiseBudget={submitRaiseBudget}
        sessionList={sessions.data}
        currentId={id}
        onPickSession={pickSession}
        historyMenu={historyMenu}
      />
      <div className="creation-stream" ref={stream}>
        <div className="creation-feed">
          <ReadFailure error={sessions.error ?? current.error} what="創作紀錄" />
          {!p && (
            <SessionEmptyState
              busy={busy}
              onPick={(prompt) => {
                setMessage(prompt);
                textarea.current?.focus();
              }}
            />
          )}

          {session && p && (
            <>
              <ConversationLog
                session={session}
                thumbs={thumbs}
                working={working}
                busy={busy}
                perform={perform}
              />
              {roundTimeline.length > 0 && <RoundTimeline items={roundTimeline} />}
              {p.brief && <BriefCard p={p} locked={locked} perform={perform} />}
              {session.state === "needs_reupload" && (
                <p className="system-line">
                  這一步中斷了，Agent 沒能讀出那張圖；請在下面重新上傳同一張。
                </p>
              )}
              {p.diagram_understanding && (
                <DiagramUnderstandingCard
                  understanding={p.diagram_understanding}
                  confirmed={p.diagram_confirmed}
                  pendingAction={p.pending_action}
                  locked={locked}
                  perform={perform}
                />
              )}
              {p.diagram_description && (
                <DiagramDescriptionCard
                  description={p.diagram_description}
                  confirmed={p.diagram_description_confirmed}
                  pendingAction={p.pending_action}
                  locked={locked}
                  perform={perform}
                />
              )}
              {p.diagram_interpretation && (
                <DiagramInterpretationCard
                  interpretation={p.diagram_interpretation}
                  confirmed={p.diagram_confirmed}
                  pendingAction={p.pending_action}
                  answers={diagramAnswers}
                  onAnswer={(uncertaintyID, answer) =>
                    setDiagramAnswers((old) => ({ ...old, [uncertaintyID]: answer }))
                  }
                  locked={locked}
                  perform={perform}
                />
              )}
              {p.pending_action === "confirm_fetch" && p.pending_fetch_url && (
                <FetchConsentCard url={p.pending_fetch_url} locked={locked} perform={perform} />
              )}
              {!!p.fetches?.length && <FetchedPages fetches={p.fetches} />}
              {(p.references.length > 0 || p.pending_action === "confirm_references") && (
                <ReferencesCard
                  references={p.references}
                  pendingAction={p.pending_action}
                  catalogChecked={p.catalog_checked}
                  locked={locked}
                  perform={perform}
                />
              )}
              {p.pending_action === "confirm_duplicate" &&
                p.duplicates &&
                p.duplicates.length > 0 && (
                  <DuplicatesCard
                    duplicates={p.duplicates}
                    contentHash={p.draft?.content_hash}
                    locked={locked}
                    perform={perform}
                  />
                )}
              {p.draft && (
                <DraftCard
                  p={p}
                  draft={p.draft}
                  state={session.state}
                  terminal={terminal}
                  latest={latest}
                  locked={locked}
                  perform={perform}
                />
              )}
            </>
          )}
        </div>
      </div>
      {!terminal && (
        <Composer
          hasSession={!!session}
          latestHidden={latestHidden}
          unseen={unseen}
          onShowLatest={showLatest}
          failureBox={failureBox}
          creditsBlocked={creditsBlocked}
          limitsFailed={!!limits.error}
          credits={credits.data}
          choices={choices}
          budget={budget}
          onBudget={setBudget}
          busy={busy}
          locked={locked}
          frozen={frozen}
          dragging={dragging}
          onDragging={setDragging}
          hasContent={hasContent}
          file={file}
          preview={preview}
          onChooseFile={chooseFile}
          onClearFile={clearFile}
          fileInput={fileInput}
          refs={refs}
          onRefs={setRefs}
          onError={setError}
          picking={picking}
          onPicking={setPicking}
          message={message}
          onMessage={setMessage}
          textarea={textarea}
          onSubmit={submit}
        />
      )}
      {terminal && failureBox && <div className="composer-dock">{failureBox}</div>}
    </div>
  );
}
