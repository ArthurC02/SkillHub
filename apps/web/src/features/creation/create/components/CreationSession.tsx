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
import { Loading } from "../../../../shared/ui/Loading";
import { sessionPhase, startGate } from "../create.model";
import { useCreationAttempt, useCreationCommands, type Perform } from "../create.commands";
import { useComposer } from "../create.composer";
import { useMessageStream } from "../create.stream";
import { SessionHeader } from "./SessionHeader";
import { SessionEmptyState } from "./SessionEmptyState";
import { SessionFeed } from "./SessionFeed";
import { Composer } from "./Composer";
import { CreationFocusPanel } from "./CreationFocus";
import { SessionWorkspaceNav } from "./SessionWorkspaceNav";
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

function useCandidateRun(testCaseID?: string, versionID?: string) {
  const runs = useRuns({
    testCaseId: testCaseID,
    skillVersionId: versionID,
    enabled: Boolean(testCaseID && versionID),
  });
  const latest = runs.data?.pages[0]?.runs.find((run) => TERMINAL_RUN_STATUSES.has(run.status));
  return {
    latest: testCaseID && versionID && (runs.isPending || runs.error) ? null : latest,
    pending: Boolean(testCaseID && versionID && runs.isPending),
    error: runs.error,
  };
}

function CandidateRunStatus({ pending, error }: { pending: boolean; error: unknown }) {
  return (
    <>
      {pending && <Loading what="候選版本的試跑結果" />}
      <ReadFailure error={error} what="候選版本的試跑結果" />
    </>
  );
}

export function CreationSession(props: CreationSessionProps) {
  const [id, setID] = useSessionSelection(props);
  const [sessionsOpen, setSessionsOpen] = useState(!props.sessionId);
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
  const candidateRun = useCandidateRun(p?.candidate?.test_case_id, p?.candidate?.version_id);
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
  const messageState = useMessageStream(id, messages.length, messages.at(-1)?.role, working);
  const { stream, latestHidden, unseen, showLatest } = messageState;
  const submit = () =>
    attempt("submit", () => composer.send(commands, session, gate.budgetCredits));
  const retry = () => {
    if (lastAttempt === "submit") void submit();
    else if (lastAttempt) void perform(...lastAttempt);
  };
  const failureBox = failureNotice({ error, busy, lastAttempt, onRetry: retry, onError: setError });
  const workspace = useRef<HTMLDivElement>(null);
  const pickSession = (next: string) => {
    setID(next);
    composer.reset();
    commands.forgetPending();
    setError(undefined);
    setSessionsOpen(false);
    queueMicrotask(() => workspace.current?.focus());
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
        sessionCount={sessions.data?.length}
        sessionsOpen={sessionsOpen}
        onToggleSessions={() => setSessionsOpen((open) => !open)}
      />
      <div className="creation-studio" data-sessions-open={sessionsOpen || undefined}>
        <SessionWorkspaceNav
          sessionList={sessions.data}
          error={sessions.error}
          currentId={id}
          busy={busy}
          onPickSession={pickSession}
        />
        <div className="creation-current" id="creation-workspace" tabIndex={-1} ref={workspace}>
          <CreationFocusPanel session={session} />
          <div className="creation-stream" ref={stream}>
            <div className="creation-feed">
              <ReadFailure error={current.error} what="創作紀錄" />
              <CandidateRunStatus pending={candidateRun.pending} error={candidateRun.error} />
              {!p && <SessionEmptyState busy={busy} onPick={composer.startFrom} />}
              {session && p && (
                <SessionFeed
                  session={session}
                  thumbs={composer.thumbs}
                  working={working}
                  busy={busy}
                  terminal={terminal}
                  locked={locked}
                  latest={candidateRun.latest}
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
      </div>
    </div>
  );
}

function failureNotice({
  error,
  busy,
  lastAttempt,
  onRetry,
  onError,
}: {
  error: unknown;
  busy: boolean;
  lastAttempt: unknown;
  onRetry: () => void;
  onError: (error: undefined) => void;
}) {
  if (!error) return null;
  return (
    <FailureToast
      error={error}
      busy={busy}
      canRetry={error instanceof TypeError && Boolean(lastAttempt)}
      onRetry={onRetry}
      onClose={() => onError(undefined)}
    />
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
