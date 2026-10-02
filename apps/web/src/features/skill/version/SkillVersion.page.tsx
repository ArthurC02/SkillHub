import { Link, useParams } from "@tanstack/react-router";
import { ApiError } from "../../../core/api/client";
import type { SkillVersionSummary } from "../../../core/api/types";
import { Loading } from "../../../shared/ui/Loading";
import { ReadFailure } from "../../../shared/ui/LoginRequired";
import { Timestamp } from "../../../shared/ui/Timestamp";
import { ListFreshness } from "../../../shared/ui/ListFreshness";
import {
  PACKAGING_BLOCKED_LABEL,
  packagingGate,
  useDownloads,
  type DownloadArtifact,
} from "../../packaging";
import { PublishPanel } from "../../publishing";
import { creationStateLabel, useCreationSessionsForVersion } from "../../creation";
import {
  IN_FLIGHT_RUN_STATUSES,
  RunVerdict,
  VersionDiff,
  runStatusLabel,
  useRuns,
  type RunListItem,
} from "../../runs";
import { SkillWorkspaceNav } from "../components/SkillWorkspaceNav";
import { useSkillDetail, useSkillVersions, skillDiffUrl } from "../skills.service";
import { VersionUpload } from "./components/VersionUpload";
import "./SkillVersion.page.css";

export function SkillVersion() {
  const { skillId, versionId } = useParams({
    from: "/skills/$skillId/versions/$versionId",
  });
  const skill = useSkillDetail(skillId);
  const versions = useSkillVersions(skillId);

  if (skill.isPending) return <Loading what="這個小工具" />;
  if (skill.error instanceof ApiError && skill.error.status === 410) {
    return <p role="alert">這個小工具已從目錄下架，內容不再提供。</p>;
  }
  if (skill.error) return <ReadFailure error={skill.error} what="這個小工具" />;
  if (!skill.data) return <p role="alert">找不到這個小工具。</p>;

  if (versions.isPending) return <Loading what="版本脈絡" />;
  if (versions.error) return <ReadFailure error={versions.error} what="版本脈絡" />;

  const list = versions.data?.versions ?? [];
  const selectedIndex = list.findIndex((version) => version.version_id === versionId);
  const selected = list[selectedIndex];

  if (!selected) {
    return (
      <section>
        <h1>無法開啟這個版本</h1>
        <p role="alert">
          {list.length === 0
            ? "無權檢視——這個工作區看不到這個小工具的版本內容；這不代表它沒有版本。"
            : "這個版本不屬於目前的小工具，或已不在這個工作區可見的版本清單中。"}
        </p>
        <Link to="/skills/$skillId" params={{ skillId }}>
          回到小工具總覽
        </Link>
      </section>
    );
  }

  const previous = list[selectedIndex + 1];
  const gate = packagingGate(skill.data);

  return (
    <article className="version-workspace">
      <header className="version-workspace-header">
        <p className="note">不可變版本</p>
        <h1>
          {skill.data.name} v{selected.version_number}
        </h1>
        <p data-role="skill-current-summary">
          <strong>小工具目前說明：</strong> {skill.data.summary}
        </p>
        <SkillWorkspaceNav skillId={skillId} versionId={versionId} />
      </header>

      <div className="version-workspace-layout">
        <div className="version-workspace-main">
          <VersionFacts version={selected} isLatest={selectedIndex === 0} />
          <VersionActions
            skillId={skillId}
            versionId={versionId}
            versionNumber={selected.version_number}
            gate={gate}
          />
          <VersionCreationContext versionId={versionId} />
          <VersionEvidence
            skillId={skillId}
            versionId={versionId}
            versionNumber={selected.version_number}
          />
          <VersionDeliverables skillId={skillId} versionId={versionId} />

          {previous && (
            <section>
              <h2>與上一版的差異</h2>
              <VersionDiff url={skillDiffUrl(skillId, previous.version_id, versionId)} />
            </section>
          )}

          <PublishPanel skill={skill.data} version={selected} isLoggedIn isOwner />
        </div>

        <VersionRail skillId={skillId} versionId={versionId} versions={list} />
      </div>
    </article>
  );
}

function VersionDeliverables({ skillId, versionId }: { skillId: string; versionId: string }) {
  const downloads = useDownloads();
  const artifacts =
    downloads.data?.downloads.filter(
      (artifact) => artifact.skill_id === skillId && artifact.skill_version_id === versionId,
    ) ?? [];

  return (
    <section aria-labelledby="version-deliverables-title">
      <p className="note">打包後的保存結果</p>
      <h2 id="version-deliverables-title">這一版的交付套件</h2>
      {downloads.isPending && <Loading what="這一版的交付套件" />}
      <ReadFailure error={downloads.error} what="這一版的交付套件">
        <p role="alert">暫時無法讀取這一版的交付套件。</p>
      </ReadFailure>
      {downloads.data &&
        (artifacts.length === 0 ? (
          <p>這一版還沒有交付套件。完成打包後，套件與保留狀態會留在這裡。</p>
        ) : (
          <ul className="download-list" data-role="evidence">
            {artifacts.map((artifact) => (
              <VersionDeliverable key={artifact.artifact_id} artifact={artifact} />
            ))}
          </ul>
        ))}
    </section>
  );
}

function VersionDeliverable({ artifact }: { artifact: DownloadArtifact }) {
  return (
    <li className="download-item" data-version-deliverable>
      <p>
        <strong>{artifact.file_name}</strong> <span className="badge">{artifact.target}</span>{" "}
        <span className="badge">{artifact.serve_state.label}</span>
      </p>
      <p className="note">
        建立於 <Timestamp at={artifact.created_at} /> · 到期時間{" "}
        <Timestamp at={artifact.expires_at} />
      </p>
      <p>
        <Link to="/workspace/downloads" search={{ artifact: artifact.artifact_id }}>
          查看交付紀錄
        </Link>
      </p>
    </li>
  );
}

function VersionCreationContext({ versionId }: { versionId: string }) {
  const { enabled: creationEnabled, sessions } = useCreationSessionsForVersion(versionId);

  if (!creationEnabled) return null;

  return (
    <section aria-labelledby="version-creation-title" className="version-creation-context">
      <p className="note">建立脈絡</p>
      <h2 id="version-creation-title">Studio 歷程</h2>
      {sessions.isPending && <Loading what="這個版本的 Studio 歷程" />}
      <ReadFailure error={sessions.error} what="這個版本的 Studio 歷程" />
      {sessions.data?.length === 0 && (
        <p>沒有仍可開啟的 Studio 會話；這個版本可能由其他方式建立，或原會話已超過保存期限。</p>
      )}
      {sessions.data && sessions.data.length > 0 && (
        <ul className="version-creation-list">
          {sessions.data.map((session) => (
            <li key={session.id}>
              <Link to="/workspace/creations" search={{ session: session.id }}>
                <strong>{session.snapshot.brief || "未命名的創作會話"}</strong>
                <span>{creationStateLabel(session.state)}</span>
                <Timestamp at={session.updated_at} />
              </Link>
            </li>
          ))}
        </ul>
      )}
    </section>
  );
}

function VersionEvidence({
  skillId,
  versionId,
  versionNumber,
}: {
  skillId: string;
  versionId: string;
  versionNumber: number;
}) {
  const runs = useRuns({ skillVersionId: versionId });
  const rows = runs.data?.pages.flatMap((page) => page.runs) ?? [];

  return (
    <section aria-labelledby="version-evidence-title">
      <p className="note">這一版留下的結果</p>
      <h2 id="version-evidence-title">驗證證據</h2>
      {runs.isPending && <Loading what="這個版本的試跑證據" />}
      <ReadFailure error={runs.error} what="這個版本的試跑證據">
        <p role="alert">暫時無法讀取這個版本的試跑證據。</p>
      </ReadFailure>
      {runs.data && (
        <ListFreshness
          inFlight={rows.some((run) => IN_FLIGHT_RUN_STATUSES.has(run.status))}
          updatedAt={runs.dataUpdatedAt}
          fetching={runs.isFetching && !runs.isFetchingNextPage}
          refetch={runs.refetch}
        />
      )}
      {runs.data &&
        (rows.length === 0 ? (
          <p>
            這個版本還沒有試跑紀錄。從{" "}
            <Link to="/lab/test-cases" search={{ skill: skillId, version: versionId }}>
              驗證 v{versionNumber}
            </Link>
            開始留下第一筆證據。
          </p>
        ) : (
          <ul className="card-list" data-role="evidence">
            {rows.map((run) => (
              <VersionRunRow key={run.run_id} run={run} />
            ))}
          </ul>
        ))}
      {runs.hasNextPage && (
        <button
          type="button"
          disabled={runs.isFetchingNextPage}
          onClick={() => runs.fetchNextPage()}
        >
          {runs.isFetchingNextPage ? "載入中…" : "載入更多證據"}
        </button>
      )}
    </section>
  );
}

function VersionRunRow({ run }: { run: RunListItem }) {
  return (
    <li className="surface-card">
      <p>
        <Link to="/runs/$runId" params={{ runId: run.run_id }}>
          查看試跑結果
        </Link>
        {run.test_case_id && (
          <>
            {" · "}
            <Link
              to="/lab/test-cases/$testCaseId"
              params={{ testCaseId: run.test_case_id }}
              search={{ version: run.skill_version_id }}
            >
              開啟這次的測試題
            </Link>
          </>
        )}
      </p>
      <p className="badge-row">
        <RunVerdict verdict={run.evaluation} />
      </p>
      <p className="badge-row">
        <span className="badge">執行狀態：{runStatusLabel(run.status)}</span>
      </p>
      {run.status_reason && <p className="note">{run.status_reason}</p>}
      <p className="note">
        建立於 <Timestamp at={run.created_at} />
        {run.finished_at ? (
          <>
            {" · "}結束於 <Timestamp at={run.finished_at} />
          </>
        ) : (
          " · 尚未結束"
        )}
        {` · Provider ${run.provider}`}
        {run.failure_class ? ` · 失敗類別 ${run.failure_class.label}` : ""}
      </p>
      {run.failure_class && <p className="note">{run.failure_class.note}</p>}
    </li>
  );
}

function VersionFacts({ version, isLatest }: { version: SkillVersionSummary; isLatest: boolean }) {
  return (
    <section className="version-facts" aria-labelledby="version-facts-title" data-role="evidence">
      <div>
        <p className="note">目前脈絡</p>
        <h2 id="version-facts-title">
          v{version.version_number}
          {isLatest ? "，最新版本" : "，歷史版本"}
        </h2>
      </div>
      <dl>
        <div>
          <dt>建立時間</dt>
          <dd>
            <Timestamp at={version.created_at} />
          </dd>
        </div>
        <div>
          <dt>內容雜湊</dt>
          <dd>
            <code>{version.content_hash}</code>
          </dd>
        </div>
      </dl>
      <p className="note">
        這一版不會被覆寫。驗證、試跑紀錄、套件與 Release 都以這個版本識別碼連回同一份內容。
      </p>
    </section>
  );
}

function VersionActions({
  skillId,
  versionId,
  versionNumber,
  gate,
}: {
  skillId: string;
  versionId: string;
  versionNumber: number;
  gate: ReturnType<typeof packagingGate>;
}) {
  return (
    <section className="version-continuation" aria-labelledby="version-continuation-title">
      <h2 id="version-continuation-title">以這一版繼續</h2>
      <div className="version-workspace-actions">
        <Link
          className="action"
          to="/lab/test-cases"
          search={{ skill: skillId, version: versionId }}
        >
          驗證 v{versionNumber}
        </Link>
        <Link
          className="action-secondary"
          to="/workspace/downloads"
          search={{ bundleVersion: versionId }}
        >
          加入 Bundle
        </Link>
        {gate ? (
          <div>
            <button type="button" disabled aria-describedby="version-package-reason">
              打包 v{versionNumber}
            </button>
            <p className="note" id="version-package-reason">
              {PACKAGING_BLOCKED_LABEL[gate]}
            </p>
          </div>
        ) : (
          <Link
            className="action-secondary"
            to="/skills/$skillId/package"
            params={{ skillId }}
            search={{ version: versionId }}
          >
            打包 v{versionNumber}
          </Link>
        )}
      </div>
    </section>
  );
}

function VersionRail({
  skillId,
  versionId,
  versions,
}: {
  skillId: string;
  versionId: string;
  versions: SkillVersionSummary[];
}) {
  return (
    <aside className="version-workspace-rail" aria-label="版本選擇與修訂">
      <section>
        <h2>切換版本</h2>
        <ol className="version-workspace-list">
          {versions.map((version) => (
            <li key={version.version_id}>
              <Link
                to="/skills/$skillId/versions/$versionId"
                params={{ skillId, versionId: version.version_id }}
                aria-current={version.version_id === versionId ? "page" : undefined}
              >
                <strong>v{version.version_number}</strong>
                <span className="note">
                  <Timestamp at={version.created_at} />
                </span>
              </Link>
            </li>
          ))}
        </ol>
      </section>
      <VersionUpload skillId={skillId} />
    </aside>
  );
}
