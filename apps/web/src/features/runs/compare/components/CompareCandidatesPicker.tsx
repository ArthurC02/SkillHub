import { Link } from "@tanstack/react-router";
import type { SkillVersionSummary } from "../../../../core/api/types";
import { Loading } from "../../../../shared/ui/Loading";
import { Timestamp } from "../../../../shared/ui/Timestamp";
import { formatAt } from "../../../../shared/ui/Timestamp.model";
import { ReadFailure } from "../../../../shared/ui/LoginRequired";
import type { RunListItem } from "../../runs.service";
import { RunVerdict } from "../../components/RunVerdict";
import { runStatusLabel } from "../../runs.model";

type CandidateListProps = {
  selfPending: boolean;
  selfError: Error | null;
  onRetrySelf: () => void;
  testCaseId: string | undefined;
  siblingsPending: boolean;
  siblingsError: Error | null;
  onRetrySiblings: () => void;
  candidates: RunListItem[];
  hasMoreCandidates: boolean;
  loadingMoreCandidates: boolean;
  onLoadMoreCandidates: () => void;
  continuationRef?: (element: HTMLButtonElement | null) => void;
  continuationRunId?: string;
  versionsPending: boolean;
  versionsError: Error | null;
  onRetryVersions: () => void;
  versions: SkillVersionSummary[];
  onPick: (id: string) => void;
};

function CandidateList({
  selfPending,
  selfError,
  onRetrySelf,
  testCaseId,
  siblingsPending,
  siblingsError,
  onRetrySiblings,
  candidates,
  hasMoreCandidates,
  loadingMoreCandidates,
  onLoadMoreCandidates,
  continuationRef,
  continuationRunId,
  versionsPending,
  versionsError,
  onRetryVersions,
  versions,
  onPick,
}: CandidateListProps) {
  if (selfPending) return <Loading what="目前這次試跑" />;
  if (selfError)
    return (
      <ReadFailure
        error={selfError}
        what="目前這次試跑"
        retry={onRetrySelf}
        focusTarget="run-comparison-heading"
      />
    );
  if (!testCaseId) {
    return <p>這次試跑的測試題已無法解析，因此無法列出同一個測試題的其他試跑紀錄。</p>;
  }
  if (siblingsPending) return <Loading what="可比較的試跑紀錄" />;
  if (siblingsError)
    return (
      <ReadFailure
        error={siblingsError}
        what="可比較的試跑紀錄"
        retry={onRetrySiblings}
        focusTarget="run-comparison-heading"
      />
    );
  if (candidates.length === 0) {
    return <p>這個測試題目前只有這一次試跑紀錄，沒有同一個測試題的其他試跑紀錄可選。</p>;
  }

  return (
    <>
      {hasMoreCandidates && (
        <p className="note">
          目前僅列出已載入的紀錄；更早的試跑可繼續載入，不能據此判定沒有其他紀錄。
        </p>
      )}
      {versionsPending && <Loading what="候選試跑紀錄的 Version 編號" />}
      <ReadFailure
        error={versionsError}
        what="候選試跑紀錄的 Version 編號"
        retry={onRetryVersions}
        focusTarget="run-comparison-heading"
      />
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
                  ref={run.run_id === continuationRunId ? continuationRef : undefined}
                  aria-label={`以 ${accessibleVersion}、建立於 ${formatAt(run.created_at)} 的試跑比較，試跑紀錄 ID ${run.run_id}`}
                  onClick={() => onPick(run.run_id)}
                >
                  與這一次比較（建立於 <Timestamp at={run.created_at} />）
                </button>
              </p>
            </li>
          );
        })}
      </ul>
      {hasMoreCandidates && (
        <button
          type="button"
          disabled={loadingMoreCandidates}
          onClick={onLoadMoreCandidates}
          ref={continuationRef}
        >
          {loadingMoreCandidates ? "載入中…" : "載入更早的試跑紀錄"}
        </button>
      )}
    </>
  );
}

export function CompareCandidatesPicker({
  selfPending,
  selfError,
  onRetrySelf,
  testCaseId,
  siblingsPending,
  siblingsError,
  onRetrySiblings,
  candidates,
  hasMoreCandidates,
  loadingMoreCandidates,
  onLoadMoreCandidates,
  continuationRef,
  continuationRunId,
  versionsPending,
  versionsError,
  onRetryVersions,
  versions,
  draft,
  onDraftChange,
  onPick,
}: CandidateListProps & {
  draft: string;
  onDraftChange: (value: string) => void;
}) {
  return (
    <>
      <CandidateList
        selfPending={selfPending}
        selfError={selfError}
        onRetrySelf={onRetrySelf}
        testCaseId={testCaseId}
        siblingsPending={siblingsPending}
        siblingsError={siblingsError}
        onRetrySiblings={onRetrySiblings}
        candidates={candidates}
        hasMoreCandidates={hasMoreCandidates}
        loadingMoreCandidates={loadingMoreCandidates}
        onLoadMoreCandidates={onLoadMoreCandidates}
        continuationRef={continuationRef}
        continuationRunId={continuationRunId}
        versionsPending={versionsPending}
        versionsError={versionsError}
        onRetryVersions={onRetryVersions}
        versions={versions}
        onPick={onPick}
      />
      <form
        onSubmit={(e) => {
          e.preventDefault();
          onPick(draft);
        }}
      >
        <label htmlFor="against">要比較的另一個試跑紀錄 ID</label>{" "}
        <input
          id="against"
          value={draft}
          onChange={(e) => onDraftChange(e.target.value)}
          size={40}
          placeholder="另一個試跑紀錄的平台 run_id"
        />{" "}
        <button type="submit">比較</button>
        <p className="note">
          {(candidates.length > 0 ? "從上面選一個同一個測試題的試跑紀錄，或" : "") +
            "輸入另一筆試跑紀錄的 ID 後開始比較。別的測試題或別的小工具的試跑紀錄也可以。"}
        </p>
      </form>
    </>
  );
}
