import { useEffect, useRef } from "react";
import { ReadFailure } from "../../../../shared/ui/LoginRequired";
import { Loading } from "../../../../shared/ui/Loading";
import { Timestamp } from "../../../../shared/ui/Timestamp";
import type { CreationSession } from "../../creation.service";
import { creationStateLabel } from "../../creation.service";

const MAX_VISIBLE_SESSIONS = 50;

export function SessionWorkspaceNav({
  sessionList,
  error,
  currentId,
  busy,
  onPickSession,
  pendingSwitch,
  onConfirmSwitch,
  onCancelSwitch,
}: {
  sessionList: CreationSession[] | undefined;
  error: unknown;
  currentId: string;
  busy: boolean;
  onPickSession: (id: string) => void;
  pendingSwitch: string | null;
  onConfirmSwitch: () => void;
  onCancelSwitch: () => void;
}) {
  const visible = sessionList?.slice(0, MAX_VISIBLE_SESSIONS);
  const confirmButton = useRef<HTMLButtonElement>(null);
  useEffect(() => {
    if (pendingSwitch !== null) confirmButton.current?.focus();
  }, [pendingSwitch]);
  return (
    <aside
      className="creation-sessions studio-session-rail"
      id="creation-sessions"
      aria-labelledby="sessions-title"
    >
      <header>
        <div>
          <span className="creation-eyebrow">Studio</span>
          <h2 id="sessions-title">近期創作</h2>
        </div>
        {visible && <span className="creation-session-count">{visible.length} 場</span>}
      </header>
      <button
        type="button"
        className="creation-new-session"
        disabled={busy}
        aria-current={!currentId ? "page" : undefined}
        onClick={() => onPickSession("")}
      >
        ＋ 開始新的創作
      </button>
      {pendingSwitch !== null && (
        <div className="notice creation-switch-confirm" role="alert">
          <p>目前有未送出的內容。切換創作會捨棄它。</p>
          <div>
            <button type="button" ref={confirmButton} onClick={onCancelSwitch}>
              繼續編輯
            </button>
            <button type="button" className="destructive" onClick={onConfirmSwitch}>
              捨棄並切換
            </button>
          </div>
        </div>
      )}
      <ReadFailure error={error} what="近期創作" />
      {!error && !sessionList && <Loading what="近期創作" />}
      {visible?.length === 0 && <p className="creation-session-empty">還沒有可續作的創作。</p>}
      {visible && visible.length > 0 && (
        <ul>
          {visible.map((session) => (
            <li key={session.id}>
              <button
                type="button"
                className="creation-session-item"
                data-session={session.id}
                disabled={busy}
                aria-current={session.id === currentId ? "page" : undefined}
                onClick={() => onPickSession(session.id)}
              >
                <span className="creation-session-name">
                  {session.snapshot.brief.slice(0, 56) || "尚未確認需求"}
                </span>
                <span className="creation-session-meta">
                  <span>
                    {session.id === currentId && <strong>目前 · </strong>}
                    {creationStateLabel(session.state)}
                  </span>
                  <Timestamp at={session.updated_at} />
                </span>
              </button>
            </li>
          ))}
        </ul>
      )}
      {sessionList && sessionList.length > MAX_VISIBLE_SESSIONS && (
        <p className="creation-session-limit">只顯示前 {MAX_VISIBLE_SESSIONS} 場。</p>
      )}
    </aside>
  );
}
