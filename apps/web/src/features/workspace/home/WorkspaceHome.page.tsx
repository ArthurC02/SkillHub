import { useEffect, useState } from "react";
import { Link } from "@tanstack/react-router";
import { useMe } from "../../../core/session/me.service";
import { Loading } from "../../../shared/ui/Loading";
import { ListFreshness } from "../../../shared/ui/ListFreshness";
import { PageReadFailure, ReadFailure } from "../../../shared/ui/LoginRequired";
import { Timestamp } from "../../../shared/ui/Timestamp";
import {
  creationStateLabel,
  useCreationEntryPoint,
  useCreationSessions,
  useGenerateEntryPoint,
  type CreationSession,
  type CreationState,
} from "../../creation";
import {
  CatalogExposure,
  useOwnPublications,
  type OwnerPublicationSummary,
} from "../../publishing";
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

  if (me.isPending) return <Loading what="工作區首頁" paused={me.fetchStatus === "paused"} />;
  if (me.error)
    return (
      <PageReadFailure
        heading="工作台"
        error={me.error}
        what="工作區首頁"
        retry={() => void me.refetch()}
        retrying={me.isFetching}
      />
    );
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
  const runs = useRuns();
  const busy = runs.fetchStatus !== "idle";
  const paused = runs.fetchStatus === "paused";
  const runData = runs.error ? undefined : runs.data;
  const runRows = runData?.pages.flatMap((page) => page.runs) ?? [];
  const attention = runRows.filter(
    (run) => runActivityGroup(run.status, run.evaluation.value) === "needs_attention",
  );
  const active = runRows.filter(
    (run) => runActivityGroup(run.status, run.evaluation.value) === "in_flight",
  );

  return (
    <section className="workspace-home">
      <WorkspaceHomeHero name={name} />

      <section className="workspace-home-section workspace-home-attention">
        <header>
          <h2 id="workspace-attention-heading" tabIndex={-1}>
            需要留意
          </h2>
          <p className="note">執行失敗、逾時或仍待判斷的結果會集中在這裡。</p>
        </header>
        {(runs.isPending || paused || (runRows.length === 0 && busy)) && (
          <Loading what="需要處理的試跑" paused={paused} />
        )}
        <ReadFailure
          error={busy ? undefined : runs.error}
          what="需要處理的試跑"
          retry={() => void runs.refetch()}
          retrying={runs.isFetching}
          focusTarget="workspace-attention-heading"
        />
        {runData &&
          (attention.length === 0 ? (
            !busy && <RunEmptyState kind="attention" hasMore={runs.hasNextPage} />
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
            paused={paused}
            refetch={async () => {
              const result = await runs.refetch();
              if (result.error) document.getElementById("workspace-attention-heading")?.focus();
            }}
          />
          {runs.error && !busy && (
            <p className="note" data-role="reason">
              目前無法確認哪些試跑仍在執行；請使用「需要留意」區的「再試一次」。
            </p>
          )}
          {runData &&
            (active.length === 0 ? (
              !busy && <RunEmptyState kind="active" hasMore={runs.hasNextPage} />
            ) : (
              <ul className="workspace-home-list">
                {active.slice(0, 3).map((run) => (
                  <RunItem key={run.run_id} run={run} action="查看進度" />
                ))}
              </ul>
            ))}
          <p className="workspace-home-more">
            <Link to="/activity">查看全部活動</Link>
          </p>
        </section>
      </div>

      <PublicationContinuations />

      <WorkspaceAssets />
    </section>
  );
}

function WorkspaceAssets() {
  const skills = useOwnSkills();
  const busy = skills.fetchStatus !== "idle";
  const paused = skills.fetchStatus === "paused";
  const skillData = skills.error ? undefined : skills.data;
  const owned = skillData?.skills.slice(0, 4) ?? [];
  return (
    <section className="workspace-home-section">
      <header className="workspace-home-section-heading">
        <div>
          <h2 id="workspace-assets-heading" tabIndex={-1}>
            你的資產
          </h2>
          <p className="note">從一個小工具開始，繼續建構、驗證或發佈。</p>
        </div>
        <Link className="action-secondary" to="/library">
          打開資產庫
        </Link>
      </header>
      {(skills.isPending || paused || (owned.length === 0 && busy)) && (
        <Loading what="你的資產" paused={paused} />
      )}
      <ReadFailure
        error={busy ? undefined : skills.error}
        what="你的資產"
        retry={() => void skills.refetch()}
        retrying={skills.isFetching}
        focusTarget="workspace-assets-heading"
      />
      {skillData &&
        (owned.length === 0 ? (
          !busy && (
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
          )
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
  );
}

function RunEmptyState({ kind, hasMore }: { kind: "attention" | "active"; hasMore: boolean }) {
  const state = kind === "attention" ? "需要你留意" : "正在執行";
  if (!hasMore) return <p>目前沒有{state}的試跑。</p>;
  return (
    <p>
      最近 50 筆沒有{state}的試跑；更早的狀態請到
      <Link to="/activity">全部活動</Link>查看。
    </p>
  );
}

function WorkspaceHomeHero({ name }: { name: string }) {
  return (
    <header className="workspace-home-hero">
      <p className="note">{name} 的 Workspace</p>
      <h1>繼續推進你的工作</h1>
      <p>先處理需要留意的結果，再續接創作與正在演進的小工具；不必先找回上次在哪一頁。</p>
    </header>
  );
}

function PublicationContinuations() {
  const publications = useOwnPublications();
  const busy = publications.fetchStatus !== "idle";
  const paused = publications.fetchStatus === "paused";
  const data = publications.error ? undefined : publications.data;
  const rows = data?.publications.slice(0, 3) ?? [];

  return (
    <section className="workspace-home-section">
      <header className="workspace-home-section-heading">
        <div>
          <h2 id="workspace-publications-heading" tabIndex={-1}>
            發佈成果
          </h2>
          <p className="note">延續已建立的 Publication，或回到它目前最新的 Release。</p>
        </div>
        <Link className="action-secondary" to="/workspace/downloads">
          查看全部發佈
        </Link>
      </header>
      {(publications.isPending || paused || (rows.length === 0 && busy)) && (
        <Loading what="發佈成果" paused={paused} />
      )}
      <ReadFailure
        error={busy ? undefined : publications.error}
        what="發佈成果"
        retry={() => void publications.refetch()}
        retrying={publications.isFetching}
        focusTarget="workspace-publications-heading"
      />
      {data &&
        (rows.length === 0 ? (
          !busy && <p>目前沒有發佈成果。</p>
        ) : (
          <ul className="workspace-home-list">
            {rows.map((publication) => (
              <PublicationItem
                key={`${publication.publisher}/${publication.name}`}
                publication={publication}
              />
            ))}
          </ul>
        ))}
    </section>
  );
}

function PublicationItem({ publication }: { publication: OwnerPublicationSummary }) {
  const release = publication.latest_release;
  return (
    <li>
      <div>
        <strong>
          {publication.publisher}/{publication.name}
        </strong>
        <p className="note">Publication 狀態：{publicationStatusLabel(publication.status)}</p>
        <CatalogExposure publication={publication} />
        {release ? (
          <p className="note">
            最新 Release：
            <Link
              to="/skills/$skillId/versions/$versionId"
              params={{ skillId: publication.skill_id, versionId: release.version_id }}
            >
              v{release.version_number}
            </Link>
            ・<Timestamp at={release.released_at} relative />
          </p>
        ) : (
          <p className="note">尚未建立 Release</p>
        )}
      </div>
      <Link
        className="action-secondary"
        to="/workspace/downloads"
        search={{ publication: `${publication.publisher}/${publication.name}` }}
      >
        管理發佈
      </Link>
    </li>
  );
}

function publicationStatusLabel(status: OwnerPublicationSummary["status"]): string {
  return status === "published" ? "已發佈" : "已下架";
}

function CreationContinuations() {
  const sessions = useCreationSessions();
  const busy = sessions.fetchStatus !== "idle";
  const paused = sessions.fetchStatus === "paused";
  const data = sessions.error ? undefined : sessions.data;
  const now = useDeadlineClock(sessions.data, sessions.dataUpdatedAt);
  const resumable =
    data
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

  if (data && resumable.length === 0 && !busy) return null;

  return (
    <section className="workspace-home-section">
      <header>
        <h2 id="workspace-creations-heading" tabIndex={-1}>
          繼續進行
        </h2>
        <p className="note">回到最近仍可操作的創作會話；草稿還不是已保存的小工具。</p>
      </header>
      <ListFreshness
        inFlight={working && !sessions.error}
        updatedAt={sessions.dataUpdatedAt}
        fetching={sessions.isFetching}
        paused={paused}
        refetch={async () => {
          const result = await sessions.refetch();
          if (result.error) document.getElementById("workspace-creations-heading")?.focus();
        }}
        subject="創作"
      />
      {(sessions.isPending || paused || (resumable.length === 0 && busy)) && (
        <Loading what="最近的創作" paused={paused} />
      )}
      <ReadFailure
        error={busy ? undefined : sessions.error}
        what="最近的創作"
        retry={() => void sessions.refetch()}
        retrying={sessions.isFetching}
        focusTarget="workspace-creations-heading"
      />
      {data && resumable.length > 0 && (
        <ul className="workspace-home-list">
          {resumable.map((session) => (
            <CreationItem key={session.id} session={session} />
          ))}
        </ul>
      )}
    </section>
  );
}

function useDeadlineClock(sessions: CreationSession[] | undefined, readAt: number): number {
  const [now, setNow] = useState(() => Date.now());
  const observedAt = Math.max(now, readAt);

  useEffect(() => {
    const next = sessions
      ?.map((session) => Date.parse(session.deadline))
      .filter((deadline) => deadline > observedAt)
      .sort((left, right) => left - right)[0];
    if (next === undefined) return;
    const timeout = window.setTimeout(
      () => setNow(Date.now()),
      Math.max(0, Math.min(next - Date.now() + 1, 60_000)),
    );
    return () => window.clearTimeout(timeout);
  }, [observedAt, sessions]);

  return observedAt;
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
