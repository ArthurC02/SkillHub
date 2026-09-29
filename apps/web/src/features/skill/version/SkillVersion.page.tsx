import { Link, useParams } from "@tanstack/react-router";
import { ApiError } from "../../../core/api/client";
import type { SkillVersionSummary } from "../../../core/api/types";
import { Loading } from "../../../shared/ui/Loading";
import { ReadFailure } from "../../../shared/ui/LoginRequired";
import { Timestamp } from "../../../shared/ui/Timestamp";
import { ListFreshness } from "../../../shared/ui/ListFreshness";
import { PACKAGING_BLOCKED_LABEL, packagingGate } from "../../packaging";
import { PublishPanel } from "../../publishing";
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

  if (skill.isPending) return <Loading what="這個 Skill" />;
  if (skill.error instanceof ApiError && skill.error.status === 410) {
    return <p role="alert">這個 Skill 已從目錄下架，內容不再提供。</p>;
  }
  if (skill.error) return <ReadFailure error={skill.error} what="這個 Skill" />;
  if (!skill.data) return <p role="alert">找不到這個 Skill。</p>;

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
            ? "無權檢視——這個工作區看不到這個 Skill 的版本內容；這不代表它沒有版本。"
            : "這個版本不屬於目前的 Skill，或已不在這個工作區可見的版本清單中。"}
        </p>
        <Link to="/skills/$skillId" params={{ skillId }}>
          回到 Skill 總覽
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
        <p>{skill.data.summary}</p>
        <SkillWorkspaceNav skillId={skillId} versionId={versionId} />
      </header>

      <div className="version-workspace-layout">
        <div className="version-workspace-main">
          <VersionFacts version={selected} isLatest={selectedIndex === 0} />
          <VersionEvidence
            skillId={skillId}
            versionId={versionId}
            versionNumber={selected.version_number}
          />
          <VersionActions
            skillId={skillId}
            versionId={versionId}
            versionNumber={selected.version_number}
            gate={gate}
          />

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
      {runs.isPending && <Loading what="這個版本的 Run 證據" />}
      <ReadFailure error={runs.error} what="這個版本的 Run 證據">
        <p role="alert">暫時無法讀取這個版本的 Run 證據。</p>
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
            這個版本還沒有 Run。從{" "}
            <Link to="/lab/test-cases" search={{ skill: skillId, version: versionId }}>
              驗證 v{versionNumber}
            </Link>
            開始留下第一筆證據。
          </p>
        ) : (
          <ul className="download-list" data-role="evidence">
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
    <li className="download-item">
      <p>
        <Link to="/runs/$runId" params={{ runId: run.run_id }}>
          查看 Run 結果
        </Link>
        {run.test_case_id && (
          <>
            {" · "}
            <Link
              to="/lab/test-cases/$testCaseId"
              params={{ testCaseId: run.test_case_id }}
              search={{ version: run.skill_version_id }}
            >
              開啟這次的 Test Case
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
        這一版不會被覆寫。驗證、Run、套件與 Release 都以這個版本識別碼連回同一份內容。
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
    <section>
      <h2>以這一版繼續</h2>
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
