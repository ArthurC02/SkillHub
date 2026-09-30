import { Link } from "@tanstack/react-router";
import type { SkillVersionSummary } from "../../../../core/api/types";
import { Loading } from "../../../../shared/ui/Loading";
import { Timestamp } from "../../../../shared/ui/Timestamp";
import { formatAt } from "../../../../shared/ui/Timestamp.model";
import { ReadFailure } from "../../../../shared/ui/LoginRequired";
import type { RunListItem } from "../../runs.service";
import { RunVerdict } from "../../components/RunVerdict";
import { runStatusLabel } from "../../runs.model";

function CandidateList({
  selfPending,
  selfError,
  testCaseId,
  siblingsPending,
  siblingsError,
  candidates,
  versionsPending,
  versionsError,
  versions,
  onPick,
}: {
  selfPending: boolean;
  selfError: Error | null;
  testCaseId: string | undefined;
  siblingsPending: boolean;
  siblingsError: Error | null;
  candidates: RunListItem[];
  versionsPending: boolean;
  versionsError: Error | null;
  versions: SkillVersionSummary[];
  onPick: (id: string) => void;
}) {
  if (selfPending) return <Loading what="目前這次 Run" />;
  if (selfError) return <ReadFailure error={selfError} what="目前這次 Run" />;
  if (!testCaseId) {
    return <p>這次 Run 的 Test Case 已無法解析，因此無法列出同一個 Test Case 的其他 Run。</p>;
  }
  if (siblingsPending) return <Loading what="可比較的 Run" />;
  if (siblingsError) return <ReadFailure error={siblingsError} what="可比較的 Run" />;
  if (candidates.length === 0) {
    return <p>這個 Test Case 目前只有這一次 Run，沒有同一個 Test Case 的其他 Run 可選。</p>;
  }

  return (
    <>
      {versionsPending && <Loading what="候選 Run 的 Version 編號" />}
      <ReadFailure error={versionsError} what="候選 Run 的 Version 編號" />
      <ul className="download-list">
        {candidates.map((run) => {
          const version = versions.find(
            (candidate) => candidate.version_id === run.skill_version_id,
          );
          const versionLabel = version ? `v${version.version_number}` : undefined;
          const accessibleVersion = versionLabel ?? `Version ID ${run.skill_version_id}`;

          return (
            <li key={run.run_id} className="download-item">
              <p>
                Version：
                <Link
                  to="/skills/$skillId/versions/$versionId"
                  params={{ skillId: run.skill_id, versionId: run.skill_version_id }}
                >
                  {versionLabel ?? (versionsPending ? "編號載入中" : "編號未知")}
                </Link>
              </p>
              {!version && !versionsPending && (
                <details>
                  <summary>Version ID</summary>
                  <code>{run.skill_version_id}</code>
                </details>
              )}
              <p className="badge-row">
                <RunVerdict verdict={run.evaluation} />
              </p>
              <p className="badge-row">
                <span className="badge">執行狀態：{runStatusLabel(run.status)}</span>
              </p>
              {run.status_reason && <p className="note">{run.status_reason}</p>}
              <p>
                <button
                  type="button"
                  aria-label={`以 ${accessibleVersion}、建立於 ${formatAt(run.created_at)} 的 Run 比較，Run ID ${run.run_id}`}
                  onClick={() => onPick(run.run_id)}
                >
                  與這一次比較（建立於 <Timestamp at={run.created_at} />）
                </button>
              </p>
            </li>
          );
        })}
      </ul>
    </>
  );
}

export function CompareCandidatesPicker({
  selfPending,
  selfError,
  testCaseId,
  siblingsPending,
  siblingsError,
  candidates,
  versionsPending,
  versionsError,
  versions,
  draft,
  onDraftChange,
  onPick,
}: {
  selfPending: boolean;
  selfError: Error | null;
  testCaseId: string | undefined;
  siblingsPending: boolean;
  siblingsError: Error | null;
  candidates: RunListItem[];
  versionsPending: boolean;
  versionsError: Error | null;
  versions: SkillVersionSummary[];
  draft: string;
  onDraftChange: (value: string) => void;
  onPick: (id: string) => void;
}) {
  return (
    <>
      <CandidateList
        selfPending={selfPending}
        selfError={selfError}
        testCaseId={testCaseId}
        siblingsPending={siblingsPending}
        siblingsError={siblingsError}
        candidates={candidates}
        versionsPending={versionsPending}
        versionsError={versionsError}
        versions={versions}
        onPick={onPick}
      />
      <form
        onSubmit={(e) => {
          e.preventDefault();
          onPick(draft);
        }}
      >
        <label htmlFor="against">要比較的另一個 Run ID</label>{" "}
        <input
          id="against"
          value={draft}
          onChange={(e) => onDraftChange(e.target.value)}
          size={40}
          placeholder="另一個 Run 的平台 run_id"
        />{" "}
        <button type="submit">比較</button>
        <p className="note">
          {(candidates.length > 0 ? "從上面選一個同一個 Test Case 的 Run，或" : "") +
            "輸入另一個 Run 的 ID 後開始比較。別的 Test Case 或別的 Skill 的 Run 也可以。"}
        </p>
      </form>
    </>
  );
}
