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
}: {
  sessionList: CreationSession[] | undefined;
  error: unknown;
  currentId: string;
  busy: boolean;
  onPickSession: (id: string) => void;
}) {
  const visible = sessionList?.slice(0, MAX_VISIBLE_SESSIONS);
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
