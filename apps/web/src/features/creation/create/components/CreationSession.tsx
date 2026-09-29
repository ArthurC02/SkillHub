import { useRef, useState } from "react";
import { ApiError } from "../../../../core/api/client";
import {
  useCreationLimits,
  useCreationSessions,
  useLiveCreationSession,
} from "../../creation.service";
import { useCredits } from "../../../../core/session/credits.service";
import { useRuns } from "../../../runs";
import { TERMINAL_RUN_STATUSES } from "../../../runs";
import { ReadFailure } from "../../../../shared/ui/LoginRequired";
import { sessionPhase, startGate } from "../create.model";
import { useCreationAttempt, useCreationCommands, type Perform } from "../create.commands";
import { useComposer } from "../create.composer";
import { useMessageStream } from "../create.stream";
import { SessionHeader } from "./SessionHeader";
import { SessionEmptyState } from "./SessionEmptyState";
import { SessionFeed } from "./SessionFeed";
import { Composer } from "./Composer";
import "./CreationSession.css";

type CreationSessionProps = {
  sessionId?: string;
  onSessionChange?: (id: string) => void;
};

function useSessionSelection({ sessionId, onSessionChange }: CreationSessionProps) {
  const [localID, setLocalID] = useState(""),
    id = sessionId ?? localID,
    setID = onSessionChange ?? setLocalID;
  return [id, setID] as const;
}

export function CreationSession(props: CreationSessionProps) {
  const [id, setID] = useSessionSelection(props);
  const [budget, setBudget] = useState(""),
    [diagramAnswers, setDiagramAnswers] = useState<Record<string, string>>({});
  const { error, setError, busy, lastAttempt, attempt } = useCreationAttempt();
  const composer = useComposer(budget, setError);
  const commands = useCreationCommands(setID);
  const sessions = useCreationSessions();
  const limits = useCreationLimits();
  const current = useLiveCreationSession(id);
  const session = current.data,
    p = session?.snapshot;
  const credits = useCredits();
  const testCaseID = p?.candidate?.test_case_id;
  const runs = useRuns({ testCaseId: testCaseID, enabled: Boolean(testCaseID) });
  const latest = runs.data?.pages[0]?.runs.find((r) => TERMINAL_RUN_STATUSES.has(r.status));
  const { terminal, working } = sessionPhase(session);
  const locked = busy || working || terminal;
  const gate = startGate(!!session, credits.data, limits.data, budget);
  const perform: Perform = async (kind, extra = {}) => {
    if (!session) return;
    await attempt([kind, extra], async () => {
      await commands.send(session, kind, extra);
      if (kind === "message") composer.setMessage("");
    });
  };
  const messages = p?.messages ?? [];
  const { stream, latestHidden, unseen, showLatest } = useMessageStream(
    id,
    messages.length,
    messages[messages.length - 1]?.role,
    working,
  );
  const submit = () =>
    attempt("submit", () => composer.send(commands, session, gate.budgetCredits));
  const retry = () => {
    if (lastAttempt === "submit") void submit();
    else if (lastAttempt) void perform(...lastAttempt);
  };
  const failureBox = !!error && (
    <FailureToast
      error={error}
      busy={busy}
      canRetry={error instanceof TypeError && !!lastAttempt}
      onRetry={retry}
      onClose={() => setError(undefined)}
    />
  );
  const historyMenu = useRef<HTMLDetailsElement>(null);
  const pickSession = (next: string) => {
    setID(next);
    composer.reset();
    commands.forgetPending();
    setError(undefined);
    if (historyMenu.current) historyMenu.current.open = false;
  };
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
        onError={setError}
        sessionList={sessions.data}
        currentId={id}
        onPickSession={pickSession}
        historyMenu={historyMenu}
      />
      <div className="creation-stream" ref={stream}>
        <div className="creation-feed">
          <ReadFailure error={sessions.error ?? current.error} what="創作紀錄" />
          {!p && <SessionEmptyState busy={busy} onPick={composer.startFrom} />}
          {session && p && (
            <SessionFeed
              session={session}
              thumbs={composer.thumbs}
              working={working}
              busy={busy}
              terminal={terminal}
              locked={locked}
              latest={latest}
              perform={perform}
              diagramAnswers={diagramAnswers}
              onDiagramAnswers={setDiagramAnswers}
            />
          )}
        </div>
      </div>
      {!terminal && (
        <Composer
          {...composer.inputs}
          hasSession={!!session}
          latestHidden={latestHidden}
          unseen={unseen}
          onShowLatest={showLatest}
          failureBox={failureBox}
          creditsBlocked={gate.creditsBlocked}
          limitsFailed={!!limits.error}
          credits={credits.data}
          choices={gate.choices}
          budget={budget}
          onBudget={setBudget}
          busy={busy}
          locked={locked}
          frozen={gate.frozen}
          onError={setError}
          onSubmit={submit}
        />
      )}
      {terminal && failureBox && <div className="composer-dock">{failureBox}</div>}
    </div>
  );
}

function FailureToast({
  error,
  busy,
  canRetry,
  onRetry,
  onClose,
}: {
  error: unknown;
  busy: boolean;
  canRetry: boolean;
  onRetry: () => void;
  onClose: () => void;
}) {
  return (
    <div className="notice notice-danger toast">
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
      {canRetry && (
        <button type="button" disabled={busy} onClick={onRetry}>
          重試
        </button>
      )}
      <button type="button" className="toast-close" aria-label="關閉" onClick={onClose}>
        ×
      </button>
    </div>
  );
}
