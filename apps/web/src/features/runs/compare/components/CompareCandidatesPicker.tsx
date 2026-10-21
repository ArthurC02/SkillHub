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
  selfPaused?: boolean;
  selfError: Error | null;
  onRetrySelf: () => void;
  testCaseId: string | undefined;
  siblingsPending: boolean;
  siblingsPaused?: boolean;
  siblingsError: Error | null;
  onRetrySiblings: () => void;
  candidates: RunListItem[];
  hasMoreCandidates: boolean;
  loadingMoreCandidates: boolean;
  onLoadMoreCandidates: () => void;
  continuationRef?: (element: HTMLButtonElement | null) => void;
  continuationRunId?: string;
  versionsPending: boolean;
  versionsPaused?: boolean;
  versionsError: Error | null;
  onRetryVersions: () => void;
  versions: SkillVersionSummary[];
  onPick: (id: string) => void;
};

function CandidateList({
  selfPending,
  selfPaused,
  selfError,
  onRetrySelf,
  testCaseId,
  siblingsPending,
  siblingsPaused,
  siblingsError,
  onRetrySiblings,
  candidates,
  hasMoreCandidates,
  loadingMoreCandidates,
  onLoadMoreCandidates,
  continuationRef,
  continuationRunId,
  versionsPending,
  versionsPaused,
  versionsError,
  onRetryVersions,
  versions,
  onPick,
}: CandidateListProps) {
  if (selfPending) return <Loading what="目前這次試跑" paused={selfPaused} />;
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
  if (siblingsPending || (siblingsPaused && siblingsError))
    return <Loading what="可比較的試跑紀錄" paused={siblingsPaused} />;
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
      {siblingsPaused && <Loading what="可比較的試跑紀錄" paused />}
      {hasMoreCandidates && (
        <p className="note">
          目前僅列出已載入的紀錄；更早的試跑可繼續載入，不能據此判定沒有其他紀錄。
        </p>
      )}
      {versionsPending && <Loading what="候選試跑紀錄的 Version 編號" paused={versionsPaused} />}
      <ReadFailure
        error={versionsError}
        what="候選試跑紀錄的 Version 編號"
        retry={onRetryVersions}
        focusTarget="run-comparison-heading"
      />
      <ul className="download-list">
        {candidates.map((run) => (
          <CandidateRow
            key={run.run_id}
            run={run}
            versions={versions}
            versionsPending={versionsPending}
            onPick={onPick}
            focusRef={run.run_id === continuationRunId ? continuationRef : undefined}
          />
        ))}
      </ul>
      {hasMoreCandidates && (
        <button
          type="button"
          disabled={loadingMoreCandidates || siblingsPaused}
          onClick={onLoadMoreCandidates}
          ref={continuationRef}
        >
          {siblingsPaused ? "等待連線…" : loadingMoreCandidates ? "載入中…" : "載入更早的試跑紀錄"}
        </button>
      )}
    </>
  );
}

function CandidateRow({
  run,
  versions,
  versionsPending,
  onPick,
  focusRef,
}: {
  run: RunListItem;
  versions: SkillVersionSummary[];
  versionsPending: boolean;
  onPick: (id: string) => void;
  focusRef?: (element: HTMLButtonElement | null) => void;
}) {
  const version = versions.find((candidate) => candidate.version_id === run.skill_version_id);
  const versionLabel = version ? `v${version.version_number}` : undefined;
  const accessibleVersion = versionLabel ?? `Version ID ${run.skill_version_id}`;

  return (
    <li className="download-item">
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
          ref={focusRef}
          aria-label={`以 ${accessibleVersion}、建立於 ${formatAt(run.created_at)} 的試跑比較，試跑紀錄 ID ${run.run_id}`}
          onClick={() => onPick(run.run_id)}
        >
          與這一次比較（建立於 <Timestamp at={run.created_at} />）
        </button>
      </p>
    </li>
  );
}

export function CompareCandidatesPicker({
  selfPending,
  selfPaused,
  selfError,
  onRetrySelf,
  testCaseId,
  siblingsPending,
  siblingsPaused,
  siblingsError,
  onRetrySiblings,
  candidates,
  hasMoreCandidates,
  loadingMoreCandidates,
  onLoadMoreCandidates,
  continuationRef,
  continuationRunId,
  versionsPending,
  versionsPaused,
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
      <details>
        <summary>直接輸入試跑紀錄 ID</summary>
        <form
          onSubmit={(e) => {
            e.preventDefault();
            onPick(draft.trim());
          }}
        >
          <label htmlFor="against">要比較的另一個試跑紀錄 ID</label>{" "}
          <input
            id="against"
            required
            pattern=".*\S.*"
            aria-describedby="run-comparison-id-help"
            value={draft}
            onChange={(e) => onDraftChange(e.target.value)}
            size={40}
            placeholder="另一個試跑紀錄的平台 run_id"
          />{" "}
          <button type="submit">比較</button>
          <p className="note" id="run-comparison-id-help">
            輸入另一筆試跑紀錄的 ID 後開始比較。ID
            不能只有空白；別的測試題或別的小工具的試跑紀錄也可以。
          </p>
        </form>
      </details>
      <CandidateList
        selfPending={selfPending}
        selfPaused={selfPaused}
        selfError={selfError}
        onRetrySelf={onRetrySelf}
        testCaseId={testCaseId}
        siblingsPending={siblingsPending}
        siblingsPaused={siblingsPaused}
        siblingsError={siblingsError}
        onRetrySiblings={onRetrySiblings}
        candidates={candidates}
        hasMoreCandidates={hasMoreCandidates}
        loadingMoreCandidates={loadingMoreCandidates}
        onLoadMoreCandidates={onLoadMoreCandidates}
        continuationRef={continuationRef}
        continuationRunId={continuationRunId}
        versionsPending={versionsPending}
        versionsPaused={versionsPaused}
        versionsError={versionsError}
        onRetryVersions={onRetryVersions}
        versions={versions}
        onPick={onPick}
      />
    </>
  );
}
