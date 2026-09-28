import { Loading } from "../../../../shared/ui/Loading";
import { Timestamp } from "../../../../shared/ui/Timestamp";
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
  onPick,
}: {
  selfPending: boolean;
  selfError: Error | null;
  testCaseId: string | undefined;
  siblingsPending: boolean;
  siblingsError: Error | null;
  candidates: RunListItem[];
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
    <ul className="download-list">
      {candidates.map((r) => (
        <li key={r.run_id} className="download-item">
          <p className="badge-row">
            <RunVerdict verdict={r.evaluation} />
          </p>
          <p className="badge-row">
            <span className="badge">執行狀態：{runStatusLabel(r.status)}</span>
          </p>
          {r.status_reason && <p className="note">{r.status_reason}</p>}
          <p>
            <button type="button" onClick={() => onPick(r.run_id)}>
              與這一次比較（建立於 <Timestamp at={r.created_at} />）
            </button>
          </p>
        </li>
      ))}
    </ul>
  );
}

export function CompareCandidatesPicker({
  selfPending,
  selfError,
  testCaseId,
  siblingsPending,
  siblingsError,
  candidates,
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
