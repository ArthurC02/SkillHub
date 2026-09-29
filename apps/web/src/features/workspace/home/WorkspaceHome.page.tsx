import { useEffect, useState } from "react";
import { Link } from "@tanstack/react-router";
import { useMe } from "../../../core/session/me.service";
import { Loading } from "../../../shared/ui/Loading";
import { ListFreshness } from "../../../shared/ui/ListFreshness";
import { LoginRequired, ReadFailure } from "../../../shared/ui/LoginRequired";
import { unauthenticated } from "../../../shared/ui/LoginRequired.model";
import { Timestamp } from "../../../shared/ui/Timestamp";
import {
  creationStateLabel,
  useCreationEntryPoint,
  useCreationSessions,
  useGenerateEntryPoint,
  type CreationSession,
  type CreationState,
} from "../../creation";
import { useOwnSkills } from "../../skill";
import {
  RunVerdict,
  RunSourceLinks,
  runActivityGroup,
  runAttentionAction,
  runStatusLabel,
  useRuns,
  type RunListItem,
} from "../../runs";
import "./WorkspaceHome.page.css";

export function WorkspaceHome() {
  const me = useMe();
  const generateExposed = useGenerateEntryPoint();
  const creationExposed = useCreationEntryPoint();

  if (me.isPending) return <Loading what="工作區首頁" />;
  if (unauthenticated(me.error)) return <LoginRequired what="工作區首頁" />;
  if (me.error) return <ReadFailure error={me.error} what="工作區首頁" />;
  if (!me.data) return <p role="alert">目前無法確認這個工作區是誰的。</p>;

  return (
    <WorkspaceHomeContent
      name={me.data.display_name || me.data.email}
      creationExposed={generateExposed && creationExposed}
    />
  );
}

function WorkspaceHomeContent({
  name,
  creationExposed,
}: {
  name: string;
  creationExposed: boolean;
}) {
  const skills = useOwnSkills();
  const runs = useRuns();
  const runRows = runs.data?.pages.flatMap((page) => page.runs) ?? [];
  const attention = runRows.filter(
    (run) => runActivityGroup(run.status, run.evaluation.value) === "needs_attention",
  );
  const active = runRows.filter(
    (run) => runActivityGroup(run.status, run.evaluation.value) === "in_flight",
  );
  const owned = skills.data?.skills.slice(0, 4) ?? [];

  return (
    <section className="workspace-home">
      <header className="workspace-home-hero">
        <p className="note">{name} 的 Workspace</p>
        <h1>繼續推進你的工作</h1>
        <p>先處理需要留意的結果，再續接創作與正在演進的小工具；不必先找回上次在哪一頁。</p>
      </header>

      <section className="workspace-home-section workspace-home-attention">
        <header>
          <h2>需要留意</h2>
          <p className="note">執行失敗、逾時或仍待判斷的結果會集中在這裡。</p>
        </header>
        {runs.isPending && <Loading what="需要處理的試跑" />}
        <ReadFailure error={runs.error} what="需要處理的試跑" />
        {runs.data &&
          (attention.length === 0 ? (
            <p>目前沒有需要你留意的試跑。</p>
          ) : (
            <ul className="workspace-home-list" data-role="evidence">
              {attention.slice(0, 3).map((run) => (
                <RunItem
                  key={run.run_id}
                  run={run}
                  action={runAttentionAction(run.status, run.evaluation.value)}
                />
              ))}
            </ul>
          ))}
      </section>

      <div className="workspace-home-grid">
        {creationExposed && <CreationContinuations />}
        <section className="workspace-home-section">
          <header>
            <h2>執行中</h2>
            <p className="note">可以離開；回來時顯示的仍是伺服器狀態。</p>
          </header>
          <ListFreshness
            inFlight={active.length > 0}
            updatedAt={runs.dataUpdatedAt}
            fetching={runs.isFetching}
            refetch={runs.refetch}
          />
          {runs.data &&
            (active.length === 0 ? (
              <p>目前沒有正在執行的試跑。</p>
            ) : (
              <ul className="workspace-home-list">
                {active.slice(0, 3).map((run) => (
                  <RunItem key={run.run_id} run={run} action="查看進度" />
                ))}
              </ul>
            ))}
          <p className="workspace-home-more">
            <Link to="/workspace/runs">查看全部試跑</Link>
          </p>
        </section>
      </div>

      <section className="workspace-home-section">
        <header className="workspace-home-section-heading">
          <div>
            <h2>你的資產</h2>
            <p className="note">從一個小工具開始，繼續建構、驗證或發佈。</p>
          </div>
          <Link className="action-secondary" to="/library">
            打開資產庫
          </Link>
        </header>
        {skills.isPending && <Loading what="你的資產" />}
        <ReadFailure error={skills.error} what="你的資產" />
        {skills.data &&
          (owned.length === 0 ? (
            <div className="workspace-home-empty">
              <p>資產庫目前是空的。先從 Catalog 找一個，或匯入現成套件。</p>
              <div className="workspace-home-actions">
                <Link className="action-secondary" to="/" search={{}}>
                  探索 Catalog
                </Link>
                <Link className="action-secondary" to="/workspace/import">
                  匯入套件
                </Link>
              </div>
            </div>
          ) : (
            <ul className="workspace-home-assets">
              {owned.map((skill) => (
                <li key={skill.skill_id}>
                  <Link to="/skills/$skillId" params={{ skillId: skill.skill_id }}>
                    <span className="workspace-home-monogram" aria-hidden="true">
                      {Array.from(skill.name)[0] ?? "?"}
                    </span>
                    <span>
                      <strong>{skill.name}</strong>
                      <span className="note">{skill.summary}</span>
                    </span>
                  </Link>
                </li>
              ))}
            </ul>
          ))}
      </section>
    </section>
  );
}

function CreationContinuations() {
  const sessions = useCreationSessions();
  const now = useDeadlineClock(sessions.data);
  const resumable =
    sessions.data
      ?.filter(
        (session) =>
          session.state !== "saved" &&
          session.state !== "cancelled" &&
          Date.parse(session.deadline) > now,
      )
      .slice(0, 3) ?? [];
  const working = resumable.some(
    (session) => session.state === "queued" || session.state === "working",
  );

  if (sessions.data && resumable.length === 0 && !sessions.error) return null;

  return (
    <section className="workspace-home-section">
      <header>
        <h2>繼續進行</h2>
        <p className="note">回到最近仍可操作的創作會話；草稿還不是已保存的小工具。</p>
      </header>
      <ListFreshness
        inFlight={working}
        updatedAt={sessions.dataUpdatedAt}
        fetching={sessions.isFetching}
        refetch={sessions.refetch}
        subject="創作"
      />
      {sessions.isPending && <Loading what="最近的創作" />}
      <ReadFailure error={sessions.error} what="最近的創作" />
      {sessions.data && (
        <ul className="workspace-home-list">
          {resumable.map((session) => (
            <CreationItem key={session.id} session={session} />
          ))}
        </ul>
      )}
    </section>
  );
}

function useDeadlineClock(sessions: CreationSession[] | undefined): number {
  const [now, setNow] = useState(() => Date.now());

  useEffect(() => {
    const next = sessions
      ?.map((session) => Date.parse(session.deadline))
      .filter((deadline) => deadline > now)
      .sort((left, right) => left - right)[0];
    if (next === undefined) return;
    const timeout = window.setTimeout(
      () => setNow(Date.now()),
      Math.max(0, Math.min(next - now + 1, 60_000)),
    );
    return () => window.clearTimeout(timeout);
  }, [now, sessions]);

  return now;
}

function CreationItem({ session }: { session: CreationSession }) {
  const brief = session.snapshot.brief.trim().slice(0, 80) || "尚未確認需求";
  return (
    <li>
      <div>
        <strong>{brief}</strong>
        <p className="note">創作狀態：{creationStateLabel(session.state)}</p>
        <p className="note">
          最後更新：
          <Timestamp at={session.updated_at} relative />
        </p>
      </div>
      <Link className="action-secondary" to="/workspace/creations" search={{ session: session.id }}>
        {creationAction(session.state)}
      </Link>
    </li>
  );
}

function creationAction(state: CreationState): string {
  if (state === "queued" || state === "working") return "查看進度";
  if (state === "failed" || state === "needs_reupload") return "查看這一步";
  return "開啟會話";
}

function RunItem({ run, action }: { run: RunListItem; action: string }) {
  return (
    <li>
      <div>
        <RunSourceLinks run={run} />
        <p className="note">執行狀態：{runStatusLabel(run.status)}</p>
        <RunVerdict verdict={run.evaluation} />
      </div>
      <Link className="action-secondary" to="/runs/$runId" params={{ runId: run.run_id }}>
        {action}
      </Link>
    </li>
  );
}
